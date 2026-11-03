package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// minDiskSize is the smallest device zpool accepts.
const minDiskSize = 64 << 20

// layouts maps a pool layout to its zpool keyword, how many disks it takes at
// least, and how many hold redundancy.
var layouts = map[string]struct {
	keyword string
	min     int
	parity  int
}{
	"stripe": {"", 1, 0},
	"mirror": {"mirror", 2, 0},
	"raidz1": {"raidz1", 3, 1},
	"raidz2": {"raidz2", 4, 2},
	"raidz3": {"raidz3", 5, 3},
}

// reservedPoolName matches what zpool keeps for itself.
var reservedPoolName = regexp.MustCompile(`^(mirror|raidz|draid|spare|log|cache|c[0-9])`)

// Disk is a whole disk this host has, and whether a pool could be made on it.
type Disk struct {
	ID         string `json:"id"`   // a stable device path: /dev/disk/by-id/…
	Name       string `json:"name"` // sdb
	Size       int64  `json:"size"`
	Model      string `json:"model,omitempty"`
	Serial     string `json:"serial,omitempty"`
	Transport  string `json:"transport,omitempty"`
	Rotational bool   `json:"rotational"`
	// InUse says why this disk is not offered; empty means it looks free.
	InUse string `json:"in_use,omitempty"`
}

// flexText reads a JSON string, number, boolean or null as text: lsblk prints
// numbers and flags as strings before util-linux 2.33 and as values after.
type flexText string

func (f *flexText) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*f = flexText(s)
		return nil
	}
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	*f = flexText(data)
	return nil
}

type lsblkDevice struct {
	Name       flexText      `json:"name"`
	Type       flexText      `json:"type"`
	Size       flexText      `json:"size"`
	Model      flexText      `json:"model"`
	Serial     flexText      `json:"serial"`
	Transport  flexText      `json:"tran"`
	Rotational flexText      `json:"rota"`
	Mountpoint flexText      `json:"mountpoint"`
	FSType     flexText      `json:"fstype"`
	Label      flexText      `json:"label"`
	PTType     flexText      `json:"pttype"`
	Children   []lsblkDevice `json:"children"`
}

// inUse says what is on a device or under it, or "". It errs on the side of
// refusing: a disk with anything on it has to be cleared by hand first. What
// uses it comes before its partitions, the less specific reason; partitions
// repeat their disk's partition table type, so that is only read on the disk.
func (d lsblkDevice) inUse() string {
	if use := d.holds(); use != "" {
		return use
	}
	switch {
	case len(d.Children) > 0:
		return "has partitions"
	case d.PTType != "":
		return "has a partition table"
	}
	return ""
}

// holds finds what uses this device or anything under it.
func (d lsblkDevice) holds() string {
	switch {
	case d.Mountpoint != "":
		return "mounted at " + string(d.Mountpoint)
	case d.FSType == "zfs_member":
		if d.Label != "" {
			return "belongs to ZFS pool " + string(d.Label)
		}
		return "belongs to a ZFS pool"
	case d.FSType == "linux_raid_member":
		return "belongs to a RAID array"
	case d.FSType == "LVM2_member":
		return "belongs to an LVM volume group"
	case d.FSType == "swap":
		return "used as swap"
	case d.FSType != "":
		return "holds a " + string(d.FSType) + " file system"
	}
	for _, child := range d.Children {
		if use := child.holds(); use != "" {
			return use
		}
	}
	return ""
}

// Disks lists the whole disks with why each cannot be used, if so. If what the
// pools are made of cannot be read it fails rather than offer a disk that may
// be in one. In TestLoopDisks mode it offers loop devices and nothing else.
func (z *ZFS) Disks(ctx context.Context) ([]Disk, error) {
	out, err := z.Run.Run(ctx, "lsblk", "-J", "-b", "-o", "NAME,TYPE,SIZE,MODEL,SERIAL,TRAN,ROTA,MOUNTPOINT,FSTYPE,LABEL,PTTYPE")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Devices []lsblkDevice `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("reading lsblk: %w", err)
	}
	members, err := z.poolDisks(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading what the pools are made of: %w", err)
	}
	stable := z.stableNames()
	disks := []Disk{}
	for _, d := range doc.Devices {
		name := string(d.Name)
		wanted := d.Type == "disk" && !strings.HasPrefix(name, "zram") && !strings.HasPrefix(name, "ram")
		if z.TestLoopDisks {
			wanted = d.Type == "loop"
		}
		if !wanted {
			continue
		}
		disk := Disk{
			ID: stable[name], Name: name, Size: num(string(d.Size)), Model: strings.TrimSpace(string(d.Model)),
			Serial: strings.TrimSpace(string(d.Serial)), Transport: string(d.Transport),
			Rotational: d.Rotational == "true" || d.Rotational == "1",
		}
		if z.TestLoopDisks {
			disk.ID = "/dev/" + name
		}
		switch use := d.inUse(); {
		case members[name] != "":
			disk.InUse = "belongs to ZFS pool " + members[name]
		case use != "":
			disk.InUse = use
		case disk.Size < minDiskSize:
			disk.InUse = "too small for a pool"
		case disk.ID == "":
			disk.InUse = "has no stable device name"
		}
		disks = append(disks, disk)
	}
	return disks, nil
}

// poolDisks maps disks in imported pools to their pool, from ZFS itself: what
// lsblk says comes from a udev database that can be behind.
func (z *ZFS) poolDisks(ctx context.Context) (map[string]string, error) {
	pools, err := z.Pools(ctx)
	if err != nil {
		return nil, err
	}
	if err := z.AddHealth(ctx, pools); err != nil {
		return nil, err
	}
	members := map[string]string{}
	var walk func(pool string, vdevs []Vdev)
	walk = func(pool string, vdevs []Vdev) {
		for _, v := range vdevs {
			if v.Disk != "" {
				members[v.Disk] = pool
			}
			walk(pool, v.Children)
		}
	}
	for _, p := range pools {
		walk(p.Name, p.Vdevs)
	}
	return members, nil
}

// stableNames maps whole disks (sdb) to a path that keeps naming them after a
// reboot, as device names may not. Readable ids come before wwn- ones.
func (z *ZFS) stableNames() map[string]string {
	names := map[string]string{}
	for _, dir := range []string{"/dev/disk/by-id", "/dev/disk/by-path"} {
		entries, err := os.ReadDir(z.Root + dir)
		if err != nil {
			continue
		}
		list := make([]string, 0, len(entries))
		for _, e := range entries {
			if !strings.Contains(e.Name(), "-part") {
				list = append(list, e.Name())
			}
		}
		sort.SliceStable(list, func(i, j int) bool {
			return !strings.HasPrefix(list[i], "wwn-") && strings.HasPrefix(list[j], "wwn-")
		})
		for _, entry := range list {
			device, err := filepath.EvalSymlinks(z.Root + path.Join(dir, entry))
			if err != nil {
				continue
			}
			if name := filepath.Base(device); names[name] == "" {
				names[name] = path.Join(dir, entry)
			}
		}
	}
	return names
}

// NewPool is a pool to create.
type NewPool struct {
	Name       string   `json:"name"`
	Layout     string   `json:"layout"` // stripe, mirror, raidz1, raidz2, raidz3
	Disks      []string `json:"disks"`  // Disk.ID
	Mountpoint string   `json:"mountpoint"`
}

// PoolPreview is what creating a pool would make.
type PoolPreview struct {
	Output   string   `json:"output"` // zpool's dry run
	Usable   int64    `json:"usable"` // about how much it would hold
	Disks    []Disk   `json:"disks"`
	Warnings []string `json:"warnings"`
}

// checkPool validates a new pool against the disks as they are now, and
// returns the command line for it.
func (z *ZFS) checkPool(ctx context.Context, p NewPool) ([]string, []Disk, error) {
	if !validPool(p.Name) || reservedPoolName.MatchString(p.Name) {
		return nil, nil, fmt.Errorf("%w pool name %q: start with a letter, use letters, digits and - _ . :, and avoid names zpool reserves such as mirror or raidz", ErrInvalid, p.Name)
	}
	layout, ok := layouts[p.Layout]
	if !ok {
		return nil, nil, fmt.Errorf("%w layout %q", ErrInvalid, p.Layout)
	}
	if len(p.Disks) < layout.min {
		return nil, nil, fmt.Errorf("%w: %s needs at least %d disks", ErrInvalid, p.Layout, layout.min)
	}
	mount := p.Mountpoint
	if mount != "" && (!strings.HasPrefix(mount, "/") || path.Clean(mount) != mount || mount == "/") {
		return nil, nil, fmt.Errorf("%w mountpoint %q: use an absolute path", ErrInvalid, mount)
	}
	if mount == "" {
		mount = "/" + p.Name
	}
	if err := z.emptyMountpoint(mount); err != nil {
		return nil, nil, err
	}
	known, err := z.Disks(ctx)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	chosen := []Disk{}
	for _, id := range p.Disks {
		if seen[id] {
			return nil, nil, fmt.Errorf("%w: %s is listed twice", ErrInvalid, id)
		}
		seen[id] = true
		i := slices.IndexFunc(known, func(d Disk) bool { return d.ID == id && id != "" })
		if i < 0 {
			return nil, nil, fmt.Errorf("%w disk %q: it is not one of this server's disks", ErrInvalid, id)
		}
		if known[i].InUse != "" {
			return nil, nil, fmt.Errorf("%w: %s %s", ErrConflict, known[i].Name, known[i].InUse)
		}
		chosen = append(chosen, known[i])
	}
	// Never -f: it is what would let zpool overwrite a disk with data on it.
	args := []string{"-o", "ashift=12"}
	if p.Mountpoint != "" {
		args = append(args, "-m", p.Mountpoint)
	}
	args = append(args, p.Name)
	if layout.keyword != "" {
		args = append(args, layout.keyword)
	}
	return append(args, p.Disks...), chosen, nil
}

// PreviewPool asks zpool for a dry run, and says what the pool would hold.
func (z *ZFS) PreviewPool(ctx context.Context, p NewPool) (PoolPreview, error) {
	args, chosen, err := z.checkPool(ctx, p)
	if err != nil {
		return PoolPreview{}, err
	}
	out, err := z.Run.Run(ctx, "zpool", append([]string{"create", "-n"}, args...)...)
	if err != nil {
		return PoolPreview{}, err
	}
	layout := layouts[p.Layout]
	smallest, largest, total := chosen[0].Size, chosen[0].Size, int64(0)
	for _, d := range chosen {
		smallest, largest = min(smallest, d.Size), max(largest, d.Size)
		total += d.Size
	}
	preview := PoolPreview{Output: strings.TrimSpace(string(out)), Disks: chosen, Warnings: []string{}}
	switch {
	case p.Layout == "stripe":
		preview.Usable = total
		preview.Warnings = append(preview.Warnings, "No redundancy: if any one disk fails, the whole pool is lost.")
	case p.Layout == "mirror":
		preview.Usable = smallest
	default:
		preview.Usable = smallest * int64(len(chosen)-layout.parity)
	}
	if p.Layout != "stripe" && smallest != largest {
		preview.Warnings = append(preview.Warnings, "The disks differ in size: each is only used up to the size of the smallest.")
	}
	return preview, nil
}

// CreatePool creates a pool. Any existing data on a disk that lsblk does not
// show makes zpool refuse, since this never forces it.
func (z *ZFS) CreatePool(ctx context.Context, p NewPool) error {
	args, _, err := z.checkPool(ctx, p)
	if err != nil {
		return err
	}
	_, err = z.Run.Run(ctx, "zpool", append([]string{"create"}, args...)...)
	return err
}
