// Package system drives the zfs and zpool commands. ZFS is the only source
// of truth: snapshots, and the retention policies stored as a user property
// on the datasets they apply to.
package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// PolicyProperty is the ZFS user property that holds a retention policy,
// inherited by child datasets the way any ZFS property is.
const PolicyProperty = "ohmyserver:snapshots"

// ZFS runs the commands.
type ZFS struct {
	Run Runner
	// Root is the file system root, "" unless testing.
	Root string
}

// Installed reports whether the zfs and zpool commands exist.
func Installed() bool {
	for _, name := range []string{"zfs", "zpool"} {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
}

// Version returns the userland version, such as zfs-2.4.4-1.
func (z *ZFS) Version(ctx context.Context) string {
	out, err := z.Run.Run(ctx, "zfs", "version")
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(first)
}

// Dataset is a filesystem or volume.
type Dataset struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Used         int64  `json:"used"`
	Available    int64  `json:"available"`
	Referenced   int64  `json:"referenced"`
	SnapshotUsed int64  `json:"snapshot_used"` // space only snapshots keep
	Mountpoint   string `json:"mountpoint,omitempty"`
	Mounted      bool   `json:"mounted"`
	Compression  string `json:"compression"`
	Quota        int64  `json:"quota"` // bytes; 0 is none
	Snapshots    int    `json:"snapshots"`
	Policy       Policy `json:"policy"`
	// PolicySource is "" with no policy, "local", or the dataset it is
	// inherited from.
	PolicySource string `json:"policy_source,omitempty"`
	// PolicyInvalid holds a property value that could not be read.
	PolicyInvalid string `json:"policy_invalid,omitempty"`
	// ScheduleError is what failed when the scheduler last applied the policy.
	ScheduleError string `json:"schedule_error,omitempty"`
}

// Datasets lists the filesystems and volumes with their policies, with
// Snapshots filled in from a listing of them.
func (z *ZFS) Datasets(ctx context.Context) ([]Dataset, error) {
	out, err := z.Run.Run(ctx, "zfs", "list", "-Hp", "-t", "filesystem,volume",
		"-o", "name,type,used,avail,refer,usedbysnapshots,mountpoint,mounted,compression,quota")
	if err != nil {
		return nil, err
	}
	list := []Dataset{}
	for _, f := range rows(out, 10) {
		d := Dataset{
			Name: f[0], Type: f[1], Used: num(f[2]), Available: num(f[3]), Referenced: num(f[4]),
			SnapshotUsed: num(f[5]), Mounted: f[7] == "yes", Compression: f[8], Quota: num(f[9]),
		}
		if strings.HasPrefix(f[6], "/") {
			d.Mountpoint = f[6]
		}
		list = append(list, d)
	}
	policies, err := z.policies(ctx)
	if err != nil {
		return nil, err
	}
	snaps, err := z.Snapshots(ctx, "")
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, s := range snaps {
		counts[s.Dataset]++
	}
	for i := range list {
		d := &list[i]
		d.Snapshots = counts[d.Name]
		p := policies[d.Name]
		d.Policy, d.PolicySource, d.PolicyInvalid = p.policy, p.source, p.invalid
	}
	return list, nil
}

// Dataset returns one dataset, or ErrNotFound.
func (z *ZFS) Dataset(ctx context.Context, name string) (Dataset, error) {
	if err := ValidDataset(name); err != nil {
		return Dataset{}, err
	}
	list, err := z.Datasets(ctx)
	if err != nil {
		return Dataset{}, err
	}
	for _, d := range list {
		if d.Name == name {
			return d, nil
		}
	}
	return Dataset{}, fmt.Errorf("%w: no dataset %s", ErrNotFound, name)
}

type datasetPolicy struct {
	policy  Policy
	source  string
	invalid string
}

func (z *ZFS) policies(ctx context.Context) (map[string]datasetPolicy, error) {
	out, err := z.Run.Run(ctx, "zfs", "get", "-Hp", "-t", "filesystem,volume", "-o", "name,value,source", PolicyProperty)
	if err != nil {
		return nil, err
	}
	policies := map[string]datasetPolicy{}
	for _, f := range rows(out, 3) {
		name, value, source := f[0], f[1], f[2]
		var p datasetPolicy
		switch {
		case source == "local":
			p.source = "local"
		case strings.HasPrefix(source, "inherited from "):
			p.source = strings.TrimPrefix(source, "inherited from ")
		default:
			continue // not set
		}
		policy, err := ParsePolicy(value)
		if err != nil {
			p.invalid = value
		}
		p.policy = policy
		policies[name] = p
	}
	return policies, nil
}

// SetPolicy stores a policy on a dataset. Its children without one of
// their own inherit it.
func (z *ZFS) SetPolicy(ctx context.Context, dataset string, policy Policy) error {
	if err := ValidDataset(dataset); err != nil {
		return err
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	_, err := z.Run.Run(ctx, "zfs", "set", PolicyProperty+"="+policy.String(), dataset)
	return err
}

// InheritPolicy removes a dataset's own policy, so it follows its parent's.
func (z *ZFS) InheritPolicy(ctx context.Context, dataset string) error {
	if err := ValidDataset(dataset); err != nil {
		return err
	}
	_, err := z.Run.Run(ctx, "zfs", "inherit", PolicyProperty, dataset)
	return err
}

// NewDataset is a filesystem to create under an existing one. Empty values
// are inherited from the parent.
type NewDataset struct {
	Parent      string `json:"parent"`
	Name        string `json:"name"` // the last part of its name
	Mountpoint  string `json:"mountpoint"`
	Compression string `json:"compression"`
	Quota       string `json:"quota"` // 500M, 2T, …
}

var (
	compressions = []string{"off", "on", "lz4", "zstd", "gzip"}
	quotaValue   = regexp.MustCompile(`^[1-9][0-9]*(\.[0-9]+)?[KMGTP]?$`)
)

// CreateDataset creates a filesystem and returns its full name. A directory
// with files where it would mount is refused first: zfs would create the
// dataset and then fail to mount it, leaving it there with the files showing
// instead of its own.
func (z *ZFS) CreateDataset(ctx context.Context, d NewDataset) (string, error) {
	if err := ValidDataset(d.Parent); err != nil {
		return "", err
	}
	if !validComponent(d.Name) {
		return "", fmt.Errorf("%w name %q: use letters, digits and - _ . : (not starting with -)", ErrInvalid, d.Name)
	}
	if d.Mountpoint != "" && (!strings.HasPrefix(d.Mountpoint, "/") || path.Clean(d.Mountpoint) != d.Mountpoint) {
		return "", fmt.Errorf("%w mountpoint %q: use an absolute path", ErrInvalid, d.Mountpoint)
	}
	if d.Compression != "" && !slices.Contains(compressions, d.Compression) {
		return "", fmt.Errorf("%w compression %q", ErrInvalid, d.Compression)
	}
	if d.Quota != "" && !quotaValue.MatchString(d.Quota) {
		return "", fmt.Errorf("%w quota %q: use a size such as 500M or 2T", ErrInvalid, d.Quota)
	}
	parent, err := z.Dataset(ctx, d.Parent)
	if err != nil {
		return "", err
	}
	if parent.Type != "filesystem" {
		return "", fmt.Errorf("%w parent: %s is not a filesystem", ErrInvalid, d.Parent)
	}
	full := d.Parent + "/" + d.Name
	mount := d.Mountpoint
	if mount == "" && parent.Mountpoint != "" {
		mount = path.Join(parent.Mountpoint, d.Name)
	}
	if mount != "" {
		if err := z.emptyMountpoint(mount); err != nil {
			return "", err
		}
	}
	args := []string{"create"}
	if d.Mountpoint != "" {
		args = append(args, "-o", "mountpoint="+d.Mountpoint)
	}
	if d.Compression != "" {
		args = append(args, "-o", "compression="+d.Compression)
	}
	if d.Quota != "" {
		args = append(args, "-o", "quota="+d.Quota)
	}
	_, err = z.Run.Run(ctx, "zfs", append(args, full)...)
	return full, err
}

// emptyMountpoint checks that nothing would be hidden by mounting at path, or
// keep the mount from happening.
func (z *ZFS) emptyMountpoint(mount string) error {
	entries, err := os.ReadDir(z.Root + mount)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w mountpoint %s: %v", ErrInvalid, mount, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w mountpoint: %s already holds files. Choose another path; a dataset cannot take over an existing folder, so its files would have to be copied in", ErrInvalid, mount)
	}
	return nil
}

// Scrub starts checking a pool's data against its checksums, or cancels that.
func (z *ZFS) Scrub(ctx context.Context, pool string, start bool) error {
	if !validPool(pool) {
		return fmt.Errorf("%w pool name %q", ErrInvalid, pool)
	}
	args := []string{"scrub"}
	if !start {
		args = append(args, "-s")
	}
	_, err := z.Run.Run(ctx, "zpool", append(args, pool)...)
	return err
}

// Snapshot is one snapshot.
type Snapshot struct {
	Name       string `json:"name"` // dataset@snapshot
	Dataset    string `json:"dataset"`
	Short      string `json:"short"` // the part after the @
	Created    int64  `json:"created"`
	Used       int64  `json:"used"`       // freed if this one alone is destroyed
	Referenced int64  `json:"referenced"` // data it gives access to
	Written    int64  `json:"written"`    // written since the previous snapshot
	Holds      int    `json:"holds"`
	Clones     string `json:"clones,omitempty"`
	Class      string `json:"class,omitempty"` // hourly, … for automatic ones
}

// Snapshots lists snapshots newest first: of one dataset, or all with an
// empty dataset. They are ordered by creation transaction, which unlike the
// creation time (in seconds) is strict within a dataset: rollback relies on
// it to know exactly which snapshots are newer.
func (z *ZFS) Snapshots(ctx context.Context, dataset string) ([]Snapshot, error) {
	args := []string{"list", "-Hp", "-t", "snapshot", "-S", "createtxg", "-o", "name,creation,used,refer,written,userrefs,clones"}
	if dataset != "" {
		if err := ValidDataset(dataset); err != nil {
			return nil, err
		}
		args = append(args, "-d", "1", dataset)
	}
	out, err := z.Run.Run(ctx, "zfs", args...)
	if err != nil {
		return nil, err
	}
	list := []Snapshot{}
	for _, f := range rows(out, 7) {
		ds, short, ok := strings.Cut(f[0], "@")
		if !ok {
			continue
		}
		s := Snapshot{
			Name: f[0], Dataset: ds, Short: short, Created: num(f[1]), Used: num(f[2]),
			Referenced: num(f[3]), Written: num(f[4]), Holds: int(num(f[5])), Class: classOf(short),
		}
		if f[6] != "-" {
			s.Clones = f[6]
		}
		list = append(list, s)
	}
	return list, nil
}

// Find returns one snapshot and those of its dataset newer than it, oldest
// of them first, or ErrNotFound.
func (z *ZFS) Find(ctx context.Context, full string) (Snapshot, []Snapshot, error) {
	dataset, _, err := SplitSnapshot(full)
	if err != nil {
		return Snapshot{}, nil, err
	}
	list, err := z.Snapshots(ctx, dataset)
	if err != nil {
		return Snapshot{}, nil, err
	}
	for i, s := range list { // newest first
		if s.Name == full {
			newer := slices.Clone(list[:i])
			slices.Reverse(newer)
			return s, newer, nil
		}
	}
	return Snapshot{}, nil, fmt.Errorf("%w: no snapshot %s", ErrNotFound, full)
}

// Create takes a snapshot of a dataset, and of its descendants when
// recursive.
func (z *ZFS) Create(ctx context.Context, dataset, name string, recursive bool) error {
	if err := ValidDataset(dataset); err != nil {
		return err
	}
	if err := ValidSnapshotName(name); err != nil {
		return err
	}
	args := []string{"snapshot"}
	if recursive {
		args = append(args, "-r")
	}
	_, err := z.Run.Run(ctx, "zfs", append(args, dataset+"@"+name)...)
	return err
}

// Destroy destroys exactly one snapshot, never with -r or -R: anything
// depending on it makes zfs refuse. Whatever else this package does,
// `zfs destroy` is only ever run here, on a name SplitSnapshot accepted.
func (z *ZFS) Destroy(ctx context.Context, full string) error {
	if _, _, err := SplitSnapshot(full); err != nil {
		return err
	}
	_, err := z.Run.Run(ctx, "zfs", "destroy", full)
	return err
}

// Rename renames a snapshot within its dataset.
func (z *ZFS) Rename(ctx context.Context, full, name string) error {
	dataset, _, err := SplitSnapshot(full)
	if err != nil {
		return err
	}
	if err := ValidSnapshotName(name); err != nil {
		return err
	}
	_, err = z.Run.Run(ctx, "zfs", "rename", full, dataset+"@"+name)
	return err
}

// Rollback returns a dataset to a snapshot, discarding what was written
// since. Rolling back past newer snapshots destroys them, so the caller
// must name exactly the ones it showed the user: if they differ from the
// real ones, nothing happens. Clones are never destroyed (no -R); zfs
// refuses if a newer snapshot has any.
func (z *ZFS) Rollback(ctx context.Context, full string, destroyNewer []string) error {
	_, newer, err := z.Find(ctx, full)
	if err != nil {
		return err
	}
	expected := make([]string, len(newer))
	for i, s := range newer {
		expected[i] = s.Name
	}
	given := slices.Sorted(slices.Values(destroyNewer))
	slices.Sort(expected)
	if !slices.Equal(given, expected) {
		return fmt.Errorf("%w: the snapshots newer than %s changed; review them and try again", ErrConflict, full)
	}
	args := []string{"rollback"}
	if len(newer) > 0 {
		args = append(args, "-r")
	}
	_, err = z.Run.Run(ctx, "zfs", append(args, full)...)
	return err
}

// Change is one path that differs from a snapshot.
type Change struct {
	Kind string `json:"kind"` // "-" removed, "+" created, "M" modified, "R" renamed
	Type string `json:"type"` // F file, / directory, @ symlink, …
	Path string `json:"path"`
	To   string `json:"to,omitempty"` // the new path of a rename
}

// Diff lists what changed in a dataset since a snapshot, up to limit
// entries, and whether there were more.
func (z *ZFS) Diff(ctx context.Context, full string, limit int) ([]Change, bool, error) {
	if _, _, err := SplitSnapshot(full); err != nil {
		return nil, false, err
	}
	changes := []Change{}
	truncated := false
	err := z.Run.Lines(ctx, func(line string) bool {
		if len(changes) == limit {
			truncated = true
			return false
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			return true
		}
		c := Change{Kind: f[0], Type: f[1], Path: unescapePath(f[2])}
		if len(f) > 3 {
			c.To = unescapePath(f[3])
		}
		changes = append(changes, c)
		return true
	}, "zfs", "diff", "-FH", full)
	return changes, truncated, err
}

// unescapePath decodes the \NNNN octal escapes zfs diff prints for spaces,
// non-ASCII bytes and such.
func unescapePath(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+5], 8, 16); err == nil && n < 256 {
				b.WriteByte(byte(n))
				i += 4
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// rows splits tab-separated output, keeping rows with exactly n fields.
func rows(out []byte, n int) [][]string {
	var result [][]string
	for line := range strings.Lines(string(out)) {
		f := strings.Split(strings.TrimRight(line, "\n"), "\t")
		if len(f) == n {
			result = append(result, f)
		}
	}
	return result
}

// num reads a parsable number; "-" and the like are 0.
func num(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// SnapshotPath is where a mounted filesystem shows a snapshot's files,
// read-only, whether or not snapdir makes .zfs visible.
func SnapshotPath(mountpoint, short string) string {
	return strings.TrimRight(mountpoint, "/") + "/.zfs/snapshot/" + short
}
