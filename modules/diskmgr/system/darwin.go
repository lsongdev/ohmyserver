package system

// macOS: disks from diskutil (whole physical disks, their partitions,
// and the APFS containers and volumes on them), file systems from df and
// mount. RAID arrays are a Linux feature (/proc/mdstat): none here.

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// decodePlist reads an XML property list into maps, slices, strings,
// int64s and bools.
func decodePlist(data []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("plist: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local != "plist" {
			return plistValue(dec, start)
		}
	}
}

func plistValue(dec *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		m := map[string]any{}
		var key string
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					if err := dec.DecodeElement(&key, &t); err != nil {
						return nil, err
					}
					continue
				}
				v, err := plistValue(dec, t)
				if err != nil {
					return nil, err
				}
				m[key] = v
			case xml.EndElement:
				return m, nil
			}
		}
	case "array":
		list := []any{}
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := plistValue(dec, t)
				if err != nil {
					return nil, err
				}
				list = append(list, v)
			case xml.EndElement:
				return list, nil
			}
		}
	case "true", "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	case "integer":
		var s string
		if err := dec.DecodeElement(&s, &start); err != nil {
			return nil, err
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n, nil
	default: // string, data, date, real: kept as text
		var s string
		if err := dec.DecodeElement(&s, &start); err != nil && err != io.EOF {
			return nil, err
		}
		return s, nil
	}
}

// pl reads plist values without type assertions everywhere.
type pl map[string]any

func (p pl) str(k string) string { s, _ := p[k].(string); return s }
func (p pl) num(k string) int64  { n, _ := p[k].(int64); return n }
func (p pl) flag(k string) bool  { b, _ := p[k].(bool); return b }
func (p pl) list(k string) []pl {
	raw, _ := p[k].([]any)
	var out []pl
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, pl(m))
		}
	}
	return out
}
func (p pl) strings(k string) []string {
	raw, _ := p[k].([]any)
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func plistDict(data []byte) (pl, error) {
	v, err := decodePlist(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("plist: top level is not a dictionary")
	}
	return pl(m), nil
}

// blocksDarwin lists the physical disks as a tree: partitions, and on an
// APFS partition the container and its volumes.
func (s *Storage) blocksDarwin(ctx context.Context) ([]Block, error) {
	all, err := s.Run.Output(ctx, nil, "diskutil", "list", "-plist")
	if err != nil {
		return nil, err
	}
	physical, err := s.Run.Output(ctx, nil, "diskutil", "list", "-plist", "physical")
	if err != nil {
		return nil, err
	}
	infos := map[string][]byte{}
	if p, err := plistDict(physical); err == nil {
		for _, name := range p.strings("WholeDisks") {
			if info, err := s.Run.Output(ctx, nil, "diskutil", "info", "-plist", name); err == nil {
				infos[name] = info
			}
		}
	}
	return parseDiskutil(all, physical, infos)
}

// A volume's snapshot (disk3s1s1) is mounted in its place; it is shown as
// its volume.
var snapshotRe = regexp.MustCompile(`^disk\d+s\d+s\d+$`)

func parseDiskutil(all, physical []byte, infos map[string][]byte) ([]Block, error) {
	list, err := plistDict(all)
	if err != nil {
		return nil, err
	}
	phys, err := plistDict(physical)
	if err != nil {
		return nil, err
	}
	// APFS containers by the partition (physical store) they live on.
	containers := map[string]pl{}
	for _, d := range list.list("AllDisksAndPartitions") {
		for _, store := range d.list("APFSPhysicalStores") {
			containers[store.str("DeviceIdentifier")] = d
		}
	}
	var result []Block
	for _, d := range phys.list("AllDisksAndPartitions") {
		name := d.str("DeviceIdentifier")
		disk := Block{Name: name, Type: "disk", Size: d.num("Size")}
		if data, ok := infos[name]; ok {
			if info, err := plistDict(data); err == nil {
				disk.Model = strings.TrimSpace(info.str("MediaName"))
				disk.Rotational = !info.flag("SolidState")
				disk.Transport = transport(info.str("BusProtocol"))
				disk.Removable = info.flag("RemovableMedia") || info.flag("Ejectable")
			}
		}
		for _, p := range d.list("Partitions") {
			part := Block{Name: p.str("DeviceIdentifier"), Type: "part", Size: p.num("Size"),
				FSType: partitionType(p.str("Content")), Label: p.str("VolumeName"), Mountpoint: p.str("MountPoint")}
			if c, ok := containers[part.Name]; ok {
				part.Children = append(part.Children, container(c))
			}
			disk.Children = append(disk.Children, part)
		}
		// A disk formatted without a partition map is its own volume.
		if mp := d.str("MountPoint"); mp != "" {
			disk.Mountpoint, disk.Label = mp, d.str("VolumeName")
		}
		result = append(result, disk)
	}
	return result, nil
}

// container is an APFS container with its volumes. A volume's size is the
// space it uses: volumes share the container's space.
func container(c pl) Block {
	b := Block{Name: c.str("DeviceIdentifier"), Type: "apfs", Size: c.num("Size")}
	snapshotMounts := map[string]string{}
	for _, v := range c.list("APFSVolumes") {
		for _, snap := range v.list("MountedSnapshots") {
			snapshotMounts[v.str("DeviceIdentifier")] = snap.str("SnapshotMountPoint")
		}
	}
	for _, v := range c.list("APFSVolumes") {
		name := v.str("DeviceIdentifier")
		if snapshotRe.MatchString(name) {
			continue
		}
		mount := v.str("MountPoint")
		if mount == "" {
			mount = snapshotMounts[name]
		}
		// macOS's own volumes (Preboot, VM, Update, the data volume behind
		// /, …) are not what a disk is used for: / stands for them all.
		if strings.HasPrefix(mount, "/System/Volumes/") || c.flag("OSInternal") {
			mount = ""
		}
		b.Children = append(b.Children, Block{Name: name, Type: "volume", Size: v.num("CapacityInUse"),
			FSType: "apfs", Label: v.str("VolumeName"), Mountpoint: mount})
	}
	return b
}

// transport names a bus the way lsblk does where they match.
func transport(bus string) string {
	switch b := strings.ToLower(bus); b {
	case "apple fabric", "pci-express", "pci", "nvme", "nvmexpress":
		return "nvme" // Apple's internal SSDs and PCIe drives are NVMe
	case "":
		return ""
	default:
		return strings.ReplaceAll(b, " ", "-") // usb, sata, thunderbolt, disk-image, …
	}
}

// partitionType shortens a partition's content type: Apple_APFS → apfs.
func partitionType(content string) string {
	switch content {
	case "EFI":
		return "efi"
	case "Microsoft Basic Data":
		return "basic-data"
	}
	return strings.ToLower(strings.TrimPrefix(content, "Apple_"))
}

// fileSystemsDarwin lists mounted file systems from df (capacity) and
// mount (types and options). macOS mounts many internal volumes hidden
// from the Finder (nobrowse); those are left out, as are pseudo file
// systems. APFS volumes share their container's space, so each container
// shows once, at its shortest mount point, with the container's use.
func (s *Storage) fileSystemsDarwin(ctx context.Context) ([]FileSystem, error) {
	df, err := s.Run.Output(ctx, nil, "df", "-k", "-P")
	if err != nil && len(df) == 0 {
		return nil, err
	}
	mount, err := s.Run.Output(ctx, nil, "mount")
	if err != nil {
		return nil, err
	}
	return parseDarwinFileSystems(df, mount), nil
}

// mountRe matches "/dev/disk3s5 on /System/Volumes/Data (apfs, local, …)".
var mountRe = regexp.MustCompile(`^(.+) on (.+) \(([^)]*)\)$`)

var apfsVolumeRe = regexp.MustCompile(`^/dev/(disk\d+)s\d+`)

func parseDarwinFileSystems(df, mount []byte) []FileSystem {
	type mountInfo struct {
		fstype string
		hidden bool
	}
	mounts := map[string]mountInfo{} // by mount point
	for _, line := range lines(mount) {
		m := mountRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		opts := strings.Split(m[3], ", ")
		mounts[m[2]] = mountInfo{fstype: opts[0], hidden: slices.Contains(opts, "nobrowse")}
	}
	seen := map[string]int{}
	var result []FileSystem
	for i, line := range lines(df) {
		f := strings.Fields(line)
		if i == 0 || len(f) < 6 {
			continue
		}
		// The mount point may hold spaces; it starts after the capacity
		// column ("35%").
		at := -1
		for j := 1; j < len(f); j++ {
			if strings.HasSuffix(f[j], "%") {
				at = j
				break
			}
		}
		if at < 4 || at+1 >= len(f) {
			continue
		}
		kb := func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n * 1024 }
		fs := FileSystem{Source: strings.Join(f[:at-3], " "), Size: kb(f[at-3]), Used: kb(f[at-2]), Available: kb(f[at-1]),
			Mountpoint: strings.Join(f[at+1:], " ")}
		info := mounts[fs.Mountpoint]
		fs.Type = info.fstype
		if fs.Size == 0 || info.hidden && fs.Mountpoint != "/" || fs.Type == "devfs" || fs.Type == "autofs" {
			continue
		}
		key := fs.Source
		if m := apfsVolumeRe.FindStringSubmatch(fs.Source); m != nil && fs.Type == "apfs" {
			key = m[1]
			fs.Source = "/dev/" + m[1] // the container
			fs.Used = fs.Size - fs.Available
		}
		if j, ok := seen[key]; ok {
			if len(fs.Mountpoint) < len(result[j].Mountpoint) {
				result[j] = fs
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, fs)
	}
	return result
}
