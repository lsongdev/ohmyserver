// Package files carries out file operations under a root directory:
// listing, reading and writing text, uploads, downloads (directories as
// tar.gz), creating folders, renaming, moving, copying and deleting.
//
// Paths are absolute as the user sees them ("/etc/hosts") and resolved
// under the root, which they can never leave. Deleting moves to a trash
// after a preview; see safety.go for what can never be deleted, moved or
// renamed.
package files

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// Errors the API maps to statuses.
var (
	ErrInvalid   = errors.New("invalid")
	ErrNotFound  = errors.New("not found")
	ErrExists    = errors.New("already exists")
	ErrProtected = errors.New("protected")
)

// Protected are directories that are never deleted, moved or renamed: the
// root and the usual top-level directories of a Linux system. Mount points,
// home directories and what contains them are refused too (see guard).
var Protected = []string{"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib32", "/lib64", "/media",
	"/mnt", "/opt", "/proc", "/root", "/run", "/sbin", "/srv", "/sys", "/tmp", "/usr", "/var"}

// FS operates on the files under Root.
type FS struct {
	Root string // "/" for the whole system

	devOf    func(string) (uint64, error) // nil: the real device IDs
	mountsOf func() []string              // nil: the mount table

	mu     sync.Mutex
	owners map[uint32]string
	groups map[uint32]string
}

// New returns an FS rooted at root.
func New(root string) (*FS, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &FS{Root: abs, owners: map[uint32]string{}, groups: map[uint32]string{}}, nil
}

// Clean validates a user path and returns it in canonical form ("/a/b").
func Clean(p string) (string, error) {
	if p == "" || !strings.HasPrefix(p, "/") || strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("%w path %q: use an absolute path", ErrInvalid, p)
	}
	return path.Clean(p), nil
}

// real returns where a user path is on disk.
func (f *FS) real(p string) (string, error) {
	p, err := Clean(p)
	if err != nil {
		return "", err
	}
	return filepath.Join(f.Root, filepath.FromSlash(p)), nil
}

func (f *FS) protected(p string) bool {
	return p == "/" || (f.Root == "/" && slices.Contains(Protected, p))
}

// validName checks a single file name, as typed for a new folder or a rename.
func validName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") || len(name) > 255 {
		return fmt.Errorf("%w name %q", ErrInvalid, name)
	}
	return nil
}

// Entry is a file or directory.
type Entry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Type     string    `json:"type"` // dir, file, link, other
	Size     int64     `json:"size"`
	Mode     string    `json:"mode"` // drwxr-xr-x
	Modified time.Time `json:"modified"`
	Owner    string    `json:"owner,omitempty"`
	Group    string    `json:"group,omitempty"`
	Target   string    `json:"target,omitempty"`    // a link's target
	TargetIs string    `json:"target_is,omitempty"` // dir or file: what a link points to; empty when broken
}

func (f *FS) entry(p string, info fs.FileInfo, real string) Entry {
	e := Entry{Name: info.Name(), Path: p, Size: info.Size(), Mode: unixMode(info.Mode()), Modified: info.ModTime().UTC()}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		e.Type = "link"
		e.Target, _ = os.Readlink(real)
		if target, err := os.Stat(real); err == nil {
			e.TargetIs = "file"
			if target.IsDir() {
				e.TargetIs = "dir"
			}
		}
	case info.IsDir():
		e.Type = "dir"
	case info.Mode().IsRegular():
		e.Type = "file"
	default:
		e.Type = "other"
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		e.Owner, e.Group = f.name(st.Uid, false), f.name(st.Gid, true)
	}
	return e
}

// name looks up a user or group name, falling back to the number.
func (f *FS) name(id uint32, group bool) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cache := f.owners
	if group {
		cache = f.groups
	}
	if n, ok := cache[id]; ok {
		return n
	}
	n := strconv.FormatUint(uint64(id), 10)
	if group {
		if g, err := user.LookupGroupId(n); err == nil {
			n = g.Name
		}
	} else if u, err := user.LookupId(n); err == nil {
		n = u.Username
	}
	cache[id] = n
	return n
}

// unixMode formats a mode as ls does: "drwxr-xr-x", "lrwxrwxrwx",
// "-rwsr-xr-x" (Go's own String differs for links, devices and set-id bits).
func unixMode(m fs.FileMode) string {
	kind := byte('-')
	switch {
	case m&fs.ModeDir != 0:
		kind = 'd'
	case m&fs.ModeSymlink != 0:
		kind = 'l'
	case m&fs.ModeCharDevice != 0:
		kind = 'c'
	case m&fs.ModeDevice != 0:
		kind = 'b'
	case m&fs.ModeNamedPipe != 0:
		kind = 'p'
	case m&fs.ModeSocket != 0:
		kind = 's'
	}
	b := []byte{kind}
	b = append(b, m.Perm().String()[1:]...)
	special := func(i int, set bool, lower, upper byte) {
		if set {
			if b[i] == 'x' {
				b[i] = lower
			} else {
				b[i] = upper
			}
		}
	}
	special(3, m&fs.ModeSetuid != 0, 's', 'S')
	special(6, m&fs.ModeSetgid != 0, 's', 'S')
	special(9, m&fs.ModeSticky != 0, 't', 'T')
	return string(b)
}

// Stat describes one path.
func (f *FS) Stat(p string) (Entry, error) {
	real, err := f.real(p)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Lstat(real)
	if err != nil {
		return Entry{}, wrap(err, p)
	}
	p, _ = Clean(p)
	e := f.entry(p, info, real)
	if p == "/" {
		e.Name = "/"
	}
	return e, nil
}

// List returns a directory's entries, directories first, then by name,
// without the trash.
func (f *FS) List(p string) ([]Entry, error) {
	real, err := f.real(p)
	if err != nil {
		return nil, err
	}
	p, _ = Clean(p)
	dir, err := os.ReadDir(real)
	if err != nil {
		return nil, wrap(err, p)
	}
	result := make([]Entry, 0, len(dir))
	for _, d := range dir {
		if d.Name() == TrashDir {
			continue // shown on its own page
		}
		info, err := d.Info()
		if err != nil {
			continue // removed meanwhile
		}
		result = append(result, f.entry(path.Join(p, d.Name()), info, filepath.Join(real, d.Name())))
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		ad, bd := a.Type == "dir" || a.TargetIs == "dir", b.Type == "dir" || b.TargetIs == "dir"
		if ad != bd {
			return ad
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return result, nil
}

// MaxText is the largest file Read returns as text.
const MaxText = 2 << 20

// Text is a file's content for viewing and editing.
type Text struct {
	Path     string `json:"path"`
	Text     string `json:"text,omitempty"`
	Size     int64  `json:"size"`
	Binary   bool   `json:"binary,omitempty"`    // not text: download it instead
	TooLarge bool   `json:"too_large,omitempty"` // over MaxText
}

// Read returns a text file's content.
func (f *FS) Read(p string) (*Text, error) {
	real, err := f.real(p)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, wrap(err, p)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a file", ErrInvalid, p)
	}
	t := &Text{Path: p, Size: info.Size()}
	if info.Size() > MaxText {
		t.TooLarge = true
		return t, nil
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return nil, wrap(err, p)
	}
	if !utf8.Valid(data) || slices.Contains(data[:min(len(data), 8192)], 0) {
		t.Binary = true
		return t, nil
	}
	t.Text = string(data)
	return t, nil
}

// Write replaces a text file's content, keeping its permissions; a new
// file gets 0644. The new content is written beside it and renamed over
// it, so a failure leaves the old file intact.
func (f *FS) Write(p, text string, create bool) error {
	real, err := f.real(p)
	if err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	info, err := os.Stat(real)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return fmt.Errorf("%w: %s is not a file", ErrInvalid, p)
	case err == nil && create:
		return fmt.Errorf("%w: %s", ErrExists, p)
	case err == nil:
		mode = info.Mode().Perm()
	case !errors.Is(err, fs.ErrNotExist) || !create:
		return wrap(err, p)
	}
	return f.replace(real, mode, info, strings.NewReader(text))
}

// replace writes r to a temporary file beside real and renames it over
// real, carrying the old file's owner over when there was one.
func (f *FS) replace(real string, mode fs.FileMode, old fs.FileInfo, r io.Reader) error {
	tmp, err := os.CreateTemp(filepath.Dir(real), "."+filepath.Base(real)+".*.part")
	if err != nil {
		return wrap(err, real)
	}
	defer os.Remove(tmp.Name()) // after a successful rename it is gone already
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	if old != nil {
		if st, ok := old.Sys().(*syscall.Stat_t); ok {
			os.Lchown(tmp.Name(), int(st.Uid), int(st.Gid)) // best effort: needs privileges
		}
	}
	return os.Rename(tmp.Name(), real)
}

// Upload saves r as name in dir. An existing file is replaced only with
// overwrite; a directory never is.
func (f *FS) Upload(dir, name string, r io.Reader, overwrite bool) (Entry, error) {
	if err := validName(name); err != nil {
		return Entry{}, err
	}
	dirReal, err := f.real(dir)
	if err != nil {
		return Entry{}, err
	}
	if info, err := os.Stat(dirReal); err != nil || !info.IsDir() {
		return Entry{}, fmt.Errorf("%w: %s is not a directory", ErrNotFound, dir)
	}
	p := path.Join(path.Clean(dir), name)
	real := filepath.Join(dirReal, name)
	old, err := os.Lstat(real)
	switch {
	case err == nil && (!overwrite || !old.Mode().IsRegular()):
		return Entry{}, fmt.Errorf("%w: %s", ErrExists, p)
	case err != nil:
		old = nil
	}
	mode := fs.FileMode(0o644)
	if old != nil {
		mode = old.Mode().Perm()
	}
	if err := f.replace(real, mode, old, r); err != nil {
		return Entry{}, err
	}
	return f.Stat(p)
}

// Open returns a file for downloading.
func (f *FS) Open(p string) (*os.File, fs.FileInfo, error) {
	real, err := f.real(p)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(real)
	if err != nil {
		return nil, nil, wrap(err, p)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

// Mkdir creates a directory (and missing parents).
func (f *FS) Mkdir(p string) error {
	real, err := f.real(p)
	if err != nil {
		return err
	}
	if err := validName(path.Base(p)); err != nil {
		return err
	}
	if _, err := os.Lstat(real); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, p)
	}
	return wrap(os.MkdirAll(real, 0o755), p)
}

// changeable resolves paths that may be deleted or moved.
func (f *FS) changeable(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: no paths", ErrInvalid)
	}
	var reals []string
	for _, p := range paths {
		clean, err := Clean(p)
		if err != nil {
			return nil, err
		}
		if err := f.guard(clean); err != nil {
			return nil, err
		}
		real, _ := f.real(clean)
		reals = append(reals, real)
	}
	return reals, nil
}

// Rename gives a path a new name in the same directory.
func (f *FS) Rename(p, name string) (Entry, error) {
	if err := validName(name); err != nil {
		return Entry{}, err
	}
	reals, err := f.changeable([]string{p})
	if err != nil {
		return Entry{}, err
	}
	clean, _ := Clean(p)
	to := path.Join(path.Dir(clean), name)
	toReal, _ := f.real(to)
	if _, err := os.Lstat(toReal); err == nil {
		return Entry{}, fmt.Errorf("%w: %s", ErrExists, to)
	}
	if err := os.Rename(reals[0], toReal); err != nil {
		return Entry{}, wrap(err, clean)
	}
	return f.Stat(to)
}

// Move moves paths into the directory dest. Across file systems it copies,
// then deletes the original.
func (f *FS) Move(paths []string, dest string) error {
	reals, err := f.changeable(paths)
	if err != nil {
		return err
	}
	targets, err := f.targets(paths, dest)
	if err != nil {
		return err
	}
	for i, real := range reals {
		err := os.Rename(real, targets[i])
		if errors.Is(err, syscall.EXDEV) {
			// Across file systems: copy, then remove the original, never
			// reaching into another file system mounted inside it.
			clean, _ := Clean(paths[i])
			if _, err = f.scan(clean, real); err == nil {
				if err = copyTree(real, targets[i]); err == nil {
					err = os.RemoveAll(real)
				}
			}
			if errors.Is(err, ErrProtected) {
				return err
			}
		}
		if err != nil {
			return wrap(err, paths[i])
		}
	}
	return nil
}

// Copy copies paths, directories recursively, into the directory dest.
func (f *FS) Copy(paths []string, dest string) error {
	targets, err := f.targets(paths, dest)
	if err != nil {
		return err
	}
	for i, p := range paths {
		real, _ := f.real(p)
		if err := copyTree(real, targets[i]); err != nil {
			return wrap(err, p)
		}
	}
	return nil
}

// targets checks dest is a directory that none of paths contains or
// already has, and returns where each path goes.
func (f *FS) targets(paths []string, dest string) ([]string, error) {
	dest, err := Clean(dest)
	if err != nil {
		return nil, err
	}
	destReal, _ := f.real(dest)
	if info, err := os.Stat(destReal); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrNotFound, dest)
	}
	var result []string
	for _, p := range paths {
		clean, err := Clean(p)
		if err != nil {
			return nil, err
		}
		real, _ := f.real(clean)
		if _, err := os.Lstat(real); err != nil {
			return nil, wrap(err, clean)
		}
		if dest == clean || strings.HasPrefix(dest+"/", clean+"/") {
			return nil, fmt.Errorf("%w: %s cannot go inside itself", ErrInvalid, clean)
		}
		target := filepath.Join(destReal, path.Base(clean))
		if _, err := os.Lstat(target); err == nil {
			return nil, fmt.Errorf("%w: %s", ErrExists, path.Join(dest, path.Base(clean)))
		}
		result = append(result, target)
	}
	return result, nil
}

// copyTree copies a file, link or directory tree, keeping permissions and
// modification times.
func copyTree(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	case info.IsDir():
		if err := os.Mkdir(to, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				return err
			}
		}
	case info.Mode().IsRegular():
		src, err := os.Open(from)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			return err
		}
		if err := dst.Close(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s is a special file", filepath.Base(from))
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

// wrap turns file system errors into this package's, naming the user path.
func wrap(err error, p string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w: %s", ErrNotFound, p)
	case errors.Is(err, fs.ErrExist):
		return fmt.Errorf("%w: %s", ErrExists, p)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("permission denied: %s", p)
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("%s: %s", p, pathErr.Err)
	}
	return err
}
