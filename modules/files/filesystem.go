package files

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrInvalidPath = errors.New("invalid filesystem path")
var ErrInvalidName = errors.New("invalid filename")

type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}
type Listing struct {
	CurrentPath string  `json:"current_path"`
	Parent      string  `json:"parent,omitempty"`
	ShowHidden  bool    `json:"show_hidden"`
	IsFile      bool    `json:"is_file"`
	File        *Entry  `json:"file,omitempty"`
	Entries     []Entry `json:"entries"`
}
type Service interface {
	List(context.Context, string, bool) (Listing, error)
	CreateFile(context.Context, string, string, string) error
	CreateFolder(context.Context, string, string) error
	Upload(context.Context, string, string, io.Reader) error
	Delete(context.Context, string) error
	Open(context.Context, string) (*os.File, os.FileInfo, error)
}

type LocalFilesystem struct {
	root       string
	capability *os.Root
}

// NewLocalFilesystem grants access to the complete local filesystem. Use
// NewLocalFilesystemAt when an embedding application should expose only a
// specific directory tree.
func NewLocalFilesystem() *LocalFilesystem {
	fs, _ := NewLocalFilesystemAt(string(filepath.Separator))
	return fs
}

// NewLocalFilesystemAt creates a filesystem capability confined to root.
// root must already exist; symlinks that resolve outside it are rejected.
func NewLocalFilesystemAt(root string) (*LocalFilesystem, error) {
	if strings.IndexByte(root, 0) >= 0 || strings.TrimSpace(root) == "" {
		return nil, ErrInvalidPath
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrInvalidPath
	}
	capability, err := os.OpenRoot(canonical)
	if err != nil {
		return nil, err
	}
	return &LocalFilesystem{root: filepath.Clean(canonical), capability: capability}, nil
}

// Close releases the OS-level directory capability. It is safe to call more
// than once; methods should not be used after closing.
func (fs *LocalFilesystem) Close() error {
	if fs.capability == nil {
		return nil
	}
	err := fs.capability.Close()
	fs.capability = nil
	return err
}

func (fs *LocalFilesystem) List(ctx context.Context, raw string, showHidden bool) (Listing, error) {
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	path, err := fs.resolvePath(raw)
	if err != nil {
		return Listing{}, err
	}
	rel, err := fs.relativePath(path)
	if err != nil {
		return Listing{}, err
	}
	info, err := fs.stat(rel, path)
	if err != nil {
		return Listing{}, err
	}
	out := Listing{CurrentPath: path, Parent: parentPath(path), ShowHidden: showHidden, Entries: make([]Entry, 0)}
	if !info.IsDir() {
		out.IsFile = true
		out.File = entryFromInfo(path, info)
		return out, nil
	}
	items, err := fs.readDir(rel, path)
	if err != nil {
		return Listing{}, err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return Listing{}, err
		}
		if !showHidden && strings.HasPrefix(item.Name(), ".") {
			continue
		}
		itemInfo, err := item.Info()
		if err != nil {
			continue
		}
		out.Entries = append(out.Entries, *entryFromInfo(filepath.Join(path, item.Name()), itemInfo))
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		if out.Entries[i].IsDir != out.Entries[j].IsDir {
			return out.Entries[i].IsDir
		}
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	return out, nil
}
func (fs *LocalFilesystem) CreateFile(ctx context.Context, dir, name, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := fs.childPath(dir, name)
	if err != nil {
		return err
	}
	return fs.writeFile(path, []byte(content), 0644)
}
func (fs *LocalFilesystem) CreateFolder(ctx context.Context, dir, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := fs.childPath(dir, name)
	if err != nil {
		return err
	}
	return fs.mkdir(path, 0755)
}
func (fs *LocalFilesystem) Upload(ctx context.Context, dir, name string, source io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := fs.childPath(dir, name)
	if err != nil {
		return err
	}
	out, err := fs.openFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, contextReader{ctx: ctx, reader: source})
	return err
}
func (fs *LocalFilesystem) Delete(ctx context.Context, raw string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(raw) == "" {
		return ErrInvalidPath
	}
	path, err := fs.resolveDeletePath(raw)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	home, _ = filepath.Abs(home)
	if filepath.Dir(path) == path || (fs.root != "" && path == fs.root) || path == filepath.Clean(home) {
		return ErrInvalidPath
	}
	rel, err := fs.relativePath(path)
	if err != nil {
		return err
	}
	if fs.capability != nil {
		return fs.capability.RemoveAll(rel)
	}
	return os.RemoveAll(path)
}
func (fs *LocalFilesystem) Open(ctx context.Context, raw string) (*os.File, os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	path, err := fs.resolvePath(raw)
	if err != nil {
		return nil, nil, err
	}
	rel, err := fs.relativePath(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := fs.stat(rel, path)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, ErrInvalidPath
	}
	file, err := fs.openFile(path, os.O_RDONLY, 0)
	return file, info, err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func (fs *LocalFilesystem) relativePath(path string) (string, error) {
	if fs.root == "" {
		return path, nil
	}
	rel, err := filepath.Rel(fs.root, path)
	if err != nil || !pathWithin(fs.root, path) {
		return "", ErrInvalidPath
	}
	if rel == "." {
		return ".", nil
	}
	return rel, nil
}

func (fs *LocalFilesystem) stat(rel, path string) (os.FileInfo, error) {
	if fs.capability != nil {
		return fs.capability.Stat(rel)
	}
	return os.Stat(path)
}

func (fs *LocalFilesystem) readDir(rel, path string) ([]os.DirEntry, error) {
	if fs.capability != nil {
		dir, err := fs.capability.Open(rel)
		if err != nil {
			return nil, err
		}
		defer dir.Close()
		return dir.ReadDir(-1)
	}
	return os.ReadDir(path)
}

func (fs *LocalFilesystem) openFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	if fs.capability != nil {
		rel, err := fs.relativePath(path)
		if err != nil {
			return nil, err
		}
		return fs.capability.OpenFile(rel, flag, perm)
	}
	return os.OpenFile(path, flag, perm)
}

func (fs *LocalFilesystem) writeFile(path string, content []byte, perm os.FileMode) error {
	file, err := fs.openFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (fs *LocalFilesystem) mkdir(path string, perm os.FileMode) error {
	if fs.capability != nil {
		rel, err := fs.relativePath(path)
		if err != nil {
			return err
		}
		return fs.capability.Mkdir(rel, perm)
	}
	return os.Mkdir(path, perm)
}
func (fs *LocalFilesystem) resolvePath(raw string) (string, error) {
	root := fs.root
	if root == "" { // Preserve the zero value as an unrestricted filesystem.
		root = string(filepath.Separator)
	}
	if strings.IndexByte(raw, 0) >= 0 {
		return "", ErrInvalidPath
	}
	if strings.TrimSpace(raw) == "" {
		raw = root
	}
	path := filepath.Clean(raw)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", ErrInvalidPath
	}
	resolved, err := resolveSymlinkPath(path)
	if err != nil {
		return "", err
	}
	if !pathWithin(root, resolved) {
		return "", ErrInvalidPath
	}
	return resolved, nil
}

func (fs *LocalFilesystem) childPath(dir, name string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.IndexByte(name, 0) >= 0 {
		return "", ErrInvalidName
	}
	parent, err := fs.resolvePath(dir)
	if err != nil {
		return "", err
	}
	return fs.resolvePath(filepath.Join(parent, name))
}

func (fs *LocalFilesystem) resolveDeletePath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || strings.IndexByte(raw, 0) >= 0 {
		return "", ErrInvalidPath
	}
	root := fs.root
	if root == "" {
		root = string(filepath.Separator)
	}
	path := filepath.Clean(raw)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", ErrInvalidPath
	}
	parent, err := fs.resolvePath(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	path = filepath.Join(parent, filepath.Base(path))
	if !pathWithin(root, path) {
		return "", ErrInvalidPath
	}
	return path, nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveSymlinkPath resolves every existing path component while allowing
// the final target (and its parents) not to exist yet for create operations.
func resolveSymlinkPath(path string) (string, error) {
	missing := make([]string, 0, 4)
	probe := filepath.Clean(path)
	for {
		_, err := os.Lstat(probe)
		if err == nil {
			resolved, evalErr := filepath.EvalSymlinks(probe)
			if evalErr != nil {
				return "", evalErr
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", err
		}
		missing = append(missing, filepath.Base(probe))
		probe = parent
	}
}
func parentPath(path string) string {
	parent := filepath.Dir(path)
	if parent == "." {
		return string(filepath.Separator)
	}
	if parent == path {
		return ""
	}
	return parent
}
func entryFromInfo(path string, info os.FileInfo) *Entry {
	return &Entry{Name: info.Name(), Path: path, Size: info.Size(), Mode: info.Mode().String(), ModTime: info.ModTime(), IsDir: info.IsDir()}
}
