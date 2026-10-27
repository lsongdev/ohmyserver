package files

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Deleting is the operation that can do the most damage, so it is never
// immediate and never blind:
//
//   - Some paths can never be deleted, moved or renamed: the root, the
//     system's top-level directories, mount points, home directories, the
//     trash itself, and any directory that contains one of them.
//   - A deletion is previewed first (files, folders, bytes) and only goes
//     ahead if what is there still matches the preview; the walk refuses to
//     cross into another file system.
//   - Deleted items move to a trash on the same file system (a rename, so
//     nothing is copied) and can be restored. Only emptying the trash removes
//     data, with the same checks.

// ErrChanged means the files changed since the preview the request is based on.
var ErrChanged = errors.New("changed since the preview")

// TrashDir is the name of the trash directory at the top of each file
// system (or of the root, if that comes first).
const TrashDir = ".ohmyserver-trash"

// device returns the ID of the device a path is on, without following a
// final symbolic link. Tests replace it to simulate mount points.
func (f *FS) device(real string) (uint64, error) {
	if f.devOf != nil {
		return f.devOf(real)
	}
	return deviceOf(real)
}

func deviceOf(real string) (uint64, error) {
	info, err := os.Lstat(real)
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("no device information on this system")
	}
	return uint64(st.Dev), nil
}

// isMount reports whether real is a mount point: on another device than
// its parent. A symbolic link never is.
func (f *FS) isMount(real string) bool {
	info, err := os.Lstat(real)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return false
	}
	parent := filepath.Dir(real)
	if parent == real {
		return true
	}
	d, err1 := f.device(real)
	pd, err2 := f.device(parent)
	return err1 == nil && err2 == nil && d != pd
}

// homes returns the user paths of home directories: those in the root's
// /etc/passwd, and /root.
func (f *FS) homes() []string {
	result := []string{"/root"}
	file, err := os.Open(filepath.Join(f.Root, "etc", "passwd"))
	if err != nil {
		return result
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 6 || !strings.HasPrefix(fields[5], "/") {
			continue
		}
		home := path.Clean(fields[5])
		if home != "/" && home != "/nonexistent" {
			result = append(result, home)
		}
	}
	return result
}

// guard refuses paths that must never be deleted, moved or renamed.
func (f *FS) guard(p string) error {
	if f.protected(p) {
		return fmt.Errorf("%w: %s is a system directory", ErrProtected, p)
	}
	if path.Base(p) == TrashDir {
		return fmt.Errorf("%w: %s is the trash; empty it from the Trash page", ErrProtected, p)
	}
	for _, home := range f.homes() {
		if p == home || strings.HasPrefix(home+"/", p+"/") {
			return fmt.Errorf("%w: %s is or contains the home directory %s", ErrProtected, p, home)
		}
	}
	real, _ := f.real(p)
	if f.isMount(real) {
		return fmt.Errorf("%w: %s is a mount point", ErrProtected, p)
	}
	return f.noMountsBelow(p, real)
}

// noMountsBelow refuses a path with a file system mounted somewhere below
// it: moving or deleting it would carry that file system along. The mount
// table answers at once; without one (not Linux) the tree is walked.
func (f *FS) noMountsBelow(p, real string) error {
	mounts, ok := f.mountPoints()
	if !ok {
		info, err := os.Lstat(real)
		if err != nil || !info.IsDir() {
			return nil
		}
		_, err = f.scan(p, real)
		if errors.Is(err, ErrProtected) {
			return err
		}
		return nil
	}
	for _, m := range mounts {
		if strings.HasPrefix(m, strings.TrimSuffix(p, "/")+"/") {
			return fmt.Errorf("%w: %s contains a mounted file system at %s", ErrProtected, p, m)
		}
	}
	return nil
}

// mountPoints returns the mount points under the root as user paths, and
// false when the mount table cannot be read.
func (f *FS) mountPoints() ([]string, bool) {
	if f.mountsOf != nil {
		return f.mountsOf(), true
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, false
	}
	var result []string
	root := strings.TrimSuffix(f.Root, "/")
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mount := unescapeMount(fields[4])
		if mount == f.Root {
			result = append(result, "/")
		} else if strings.HasPrefix(mount, root+"/") {
			result = append(result, mount[len(root):])
		}
	}
	return result, true
}

// Plan is what deleting some paths affects.
type Plan struct {
	Files int64      `json:"files"` // files, links and other non-directories
	Dirs  int64      `json:"dirs"`
	Bytes int64      `json:"bytes"`
	Items []PlanItem `json:"items"`
}

// PlanItem is one path of a Plan.
type PlanItem struct {
	Path  string `json:"path"`
	Files int64  `json:"files"`
	Dirs  int64  `json:"dirs"`
	Bytes int64  `json:"bytes"`
}

// scan counts what is under real, refusing to cross into another file
// system: deleting there would reach data on another disk.
func (f *FS) scan(p, real string) (PlanItem, error) {
	item := PlanItem{Path: p}
	root, err := f.device(real)
	if err != nil {
		return item, wrap(err, p)
	}
	err = filepath.WalkDir(real, func(walked string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			item.Files++
			if info, err := d.Info(); err == nil {
				item.Bytes += info.Size()
			}
			return nil
		}
		item.Dirs++
		if dev, err := f.device(walked); err != nil || dev != root {
			rel, _ := filepath.Rel(f.Root, walked)
			return fmt.Errorf("%w: %s contains a mounted file system at /%s", ErrProtected, p, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrProtected) {
		err = wrap(err, p)
	}
	return item, err
}

// Preview checks paths can be deleted and counts what they hold.
func (f *FS) Preview(paths []string) (*Plan, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: no paths", ErrInvalid)
	}
	plan := &Plan{}
	seen := map[string]bool{}
	for _, p := range paths {
		clean, err := Clean(p)
		if err != nil {
			return nil, err
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		if err := f.guard(clean); err != nil {
			return nil, err
		}
		real, _ := f.real(clean)
		item, err := f.scan(clean, real)
		if err != nil {
			return nil, err
		}
		plan.Files += item.Files
		plan.Dirs += item.Dirs
		plan.Bytes += item.Bytes
		plan.Items = append(plan.Items, item)
	}
	return plan, nil
}

// Delete moves paths to the trash, provided they still hold what the
// preview counted (files and folders).
func (f *FS) Delete(paths []string, expect Plan) error {
	plan, err := f.Preview(paths)
	if err != nil {
		return err
	}
	if plan.Files != expect.Files || plan.Dirs != expect.Dirs {
		return fmt.Errorf("%w: now %d files and %d folders, the preview had %d and %d; review it again",
			ErrChanged, plan.Files, plan.Dirs, expect.Files, expect.Dirs)
	}
	for _, item := range plan.Items {
		if err := f.trash(item); err != nil {
			return err
		}
	}
	return nil
}

// trashEntry is the record kept beside a trashed item.
type trashEntry struct {
	Original  string    `json:"original"`
	DeletedAt time.Time `json:"deleted_at"`
	Files     int64     `json:"files"`
	Dirs      int64     `json:"dirs"`
	Bytes     int64     `json:"bytes"`
}

// trashFor returns the trash directory for real: at the top of its file
// system, or at the root if that comes first, so moving there is a rename.
func (f *FS) trashFor(real string) (string, error) {
	dev, err := f.device(real)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(real)
	for dir != f.Root && len(dir) > len(f.Root) {
		parent := filepath.Dir(dir)
		if pd, err := f.device(parent); err != nil || pd != dev {
			break
		}
		dir = parent
	}
	return filepath.Join(dir, TrashDir), nil
}

func (f *FS) trash(item PlanItem) error {
	real, _ := f.real(item.Path)
	dir, err := f.trashFor(real)
	if err != nil {
		return wrap(err, item.Path)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		rel, _ := filepath.Rel(f.Root, dir)
		return fmt.Errorf("cannot create the trash at /%s, so %s was left in place: %v", filepath.ToSlash(rel), item.Path, errors.Unwrap(err))
	}
	name := fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(real))
	record, _ := json.Marshal(trashEntry{Original: item.Path, DeletedAt: time.Now().UTC(), Files: item.Files, Dirs: item.Dirs, Bytes: item.Bytes})
	if err := os.WriteFile(filepath.Join(dir, name+".json"), record, 0o600); err != nil {
		return wrap(err, item.Path)
	}
	if err := os.Rename(real, filepath.Join(dir, name)); err != nil {
		os.Remove(filepath.Join(dir, name+".json"))
		return wrap(err, item.Path)
	}
	return nil
}

// TrashItem is something in a trash.
type TrashItem struct {
	ID        string    `json:"id"` // the item's path in the trash
	Name      string    `json:"name"`
	Original  string    `json:"original"`
	DeletedAt time.Time `json:"deleted_at"`
	Files     int64     `json:"files"`
	Dirs      int64     `json:"dirs"`
	Bytes     int64     `json:"bytes"`
}

// trashDirs lists the trash directories under the root: at the root and
// at the top of each file system mounted below it.
func (f *FS) trashDirs() []string {
	dirs := []string{filepath.Join(f.Root, TrashDir)}
	mounts, _ := f.mountPoints()
	for _, m := range mounts {
		real, _ := f.real(m)
		dirs = append(dirs, filepath.Join(real, TrashDir))
	}
	return dirs
}

// unescapeMount decodes mountinfo's octal escapes ("\040" for a space).
func unescapeMount(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Trash lists the trashed items, newest first.
func (f *FS) Trash() ([]TrashItem, error) {
	result := []TrashItem{}
	seen := map[string]bool{}
	for _, dir := range f.trashDirs() {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			item := TrashItem{Name: e.Name()}
			if data, err := os.ReadFile(filepath.Join(dir, e.Name()+".json")); err == nil {
				var t trashEntry
				if json.Unmarshal(data, &t) == nil {
					item.Original, item.DeletedAt, item.Files, item.Dirs, item.Bytes = t.Original, t.DeletedAt, t.Files, t.Dirs, t.Bytes
				}
			}
			if _, name, ok := strings.Cut(e.Name(), "-"); ok {
				item.Name = name
			}
			rel, _ := filepath.Rel(f.Root, filepath.Join(dir, e.Name()))
			item.ID = "/" + filepath.ToSlash(rel)
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DeletedAt.After(result[j].DeletedAt) })
	return result, nil
}

// trashed resolves the ID of an item directly inside a trash directory.
func (f *FS) trashed(id string) (string, error) {
	clean, err := Clean(id)
	if err != nil {
		return "", err
	}
	if path.Base(path.Dir(clean)) != TrashDir || strings.HasSuffix(clean, ".json") {
		return "", fmt.Errorf("%w: %s is not in the trash", ErrInvalid, clean)
	}
	real, _ := f.real(clean)
	if _, err := os.Lstat(real); err != nil {
		return "", wrap(err, clean)
	}
	return real, nil
}

// Restore moves trashed items back where they were. An item whose
// original place is taken stays in the trash.
func (f *FS) Restore(ids []string) error {
	for _, id := range ids {
		real, err := f.trashed(id)
		if err != nil {
			return err
		}
		var t trashEntry
		data, err := os.ReadFile(real + ".json")
		if err == nil {
			err = json.Unmarshal(data, &t)
		}
		if err != nil || t.Original == "" {
			return fmt.Errorf("%w: %s has no record of where it came from", ErrInvalid, id)
		}
		original, err := f.real(t.Original)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(original); err == nil {
			return fmt.Errorf("%w: %s; rename or move what is there first", ErrExists, t.Original)
		}
		if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
			return wrap(err, t.Original)
		}
		if err := os.Rename(real, original); err != nil {
			return wrap(err, t.Original)
		}
		os.Remove(real + ".json")
	}
	return nil
}

// PurgePreview counts what permanently deleting trashed items removes.
func (f *FS) PurgePreview(ids []string) (*Plan, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: no items", ErrInvalid)
	}
	plan := &Plan{}
	for _, id := range ids {
		real, err := f.trashed(id)
		if err != nil {
			return nil, err
		}
		clean, _ := Clean(id)
		item, err := f.scan(clean, real)
		if err != nil {
			return nil, err
		}
		plan.Files += item.Files
		plan.Dirs += item.Dirs
		plan.Bytes += item.Bytes
		plan.Items = append(plan.Items, item)
	}
	return plan, nil
}

// Purge permanently deletes trashed items, provided they still hold what
// the preview counted.
func (f *FS) Purge(ids []string, expect Plan) error {
	plan, err := f.PurgePreview(ids)
	if err != nil {
		return err
	}
	if plan.Files != expect.Files || plan.Dirs != expect.Dirs {
		return fmt.Errorf("%w: review it again", ErrChanged)
	}
	for _, item := range plan.Items {
		real, _ := f.real(item.Path)
		if err := os.RemoveAll(real); err != nil {
			return wrap(err, item.Path)
		}
		os.Remove(real + ".json")
	}
	return nil
}
