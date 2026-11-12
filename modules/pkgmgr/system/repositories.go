package system

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf8"
)

const repositoryLimit = 128 << 10

var (
	ErrRepositoryUnsupported = errors.New("repository files are supported for APT and DNF")
	ErrRepositoryConflict    = errors.New("repository file already exists or has changed; reload it and try again")
	repositoryName           = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,119}$`)
)

// RepositoryFile represents a native package manager configuration file.
type RepositoryFile struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Removable bool   `json:"removable"`
	Content   string `json:"content"`
	Revision  string `json:"revision,omitempty"`
}

type Repositories struct {
	Root    string
	Manager string
}

func (r Repositories) directory() string {
	switch r.Manager {
	case "apt":
		return "/etc/apt/sources.list.d"
	case "dnf":
		return "/etc/yum.repos.d"
	default:
		return ""
	}
}

func (r Repositories) validName(name string) bool {
	if !repositoryName.MatchString(name) {
		return false
	}
	extension := filepath.Ext(name)
	if r.Manager == "apt" {
		return extension == ".list" || extension == ".sources"
	}
	return r.Manager == "dnf" && extension == ".repo"
}

func (r Repositories) file(id string) (RepositoryFile, error) {
	if r.directory() == "" {
		return RepositoryFile{}, ErrRepositoryUnsupported
	}
	if r.Manager == "apt" && id == "main" {
		return RepositoryFile{ID: id, Path: "/etc/apt/sources.list"}, nil
	}
	if !r.validName(id) {
		return RepositoryFile{}, fmt.Errorf("%w repository filename for %s", ErrInvalid, r.Manager)
	}
	return RepositoryFile{ID: id, Path: filepath.Join(r.directory(), id), Removable: true}, nil
}

func (r Repositories) List() ([]RepositoryFile, error) {
	dir := r.directory()
	if dir == "" {
		return nil, ErrRepositoryUnsupported
	}
	var files []RepositoryFile
	if r.Manager == "apt" {
		main, _ := r.file("main")
		info, err := os.Lstat(filepath.Join(r.Root, main.Path))
		if err == nil && info.Mode().IsRegular() {
			files = append(files, main)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	entries, err := os.ReadDir(filepath.Join(r.Root, dir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && r.validName(entry.Name()) {
			file, _ := r.file(entry.Name())
			files = append(files, file)
		}
	}
	return files, nil
}

func (r Repositories) Read(id string) (RepositoryFile, error) {
	file, err := r.file(id)
	if err != nil {
		return file, err
	}
	file, _, err = r.read(file)
	return file, err
}

func (r Repositories) read(file RepositoryFile) (RepositoryFile, fs.FileInfo, error) {
	path := filepath.Join(r.Root, file.Path)
	input, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return file, nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return file, nil, err
	}
	if !info.Mode().IsRegular() {
		return file, nil, fmt.Errorf("%w: repository path is not a regular file", ErrInvalid)
	}
	data, err := io.ReadAll(io.LimitReader(input, repositoryLimit+1))
	if err != nil {
		return file, nil, err
	}
	if err := validateRepositoryText(string(data)); err != nil {
		return file, nil, err
	}
	file.Content = string(data)
	file.Revision = fmt.Sprintf("%x", sha256.Sum256(data))
	return file, info, nil
}

func validateRepositoryText(content string) error {
	if len(content) > repositoryLimit || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return fmt.Errorf("%w: repository content must be UTF-8 text up to 128 KiB", ErrInvalid)
	}
	return nil
}

// Write creates a file when revision is empty, or replaces a matching revision.
func (r Repositories) Write(id, content, revision string) error {
	file, err := r.file(id)
	if err != nil {
		return err
	}
	if err := validateRepositoryText(content); err != nil {
		return err
	}
	path := filepath.Join(r.Root, file.Path)
	create := revision == ""
	var info fs.FileInfo
	if create {
		if !file.Removable {
			return fmt.Errorf("%w: use a filename in the repository directory", ErrInvalid)
		}
	} else {
		current, currentInfo, err := r.read(file)
		if err != nil {
			return err
		}
		if current.Revision != revision {
			return ErrRepositoryConflict
		}
		info = currentInfo
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".ohmyserver-repo-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err := io.WriteString(temp, content); err != nil {
		return err
	}
	// New files can contain repository credentials; only the owner may read them.
	if info != nil {
		stat := info.Sys().(*syscall.Stat_t)
		if err := temp.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}
		if err := temp.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if create {
		if err := os.Link(temp.Name(), path); errors.Is(err, fs.ErrExist) {
			return ErrRepositoryConflict
		} else {
			return err
		}
	}
	return os.Rename(temp.Name(), path)
}

func (r Repositories) Delete(id, revision string) error {
	file, err := r.file(id)
	if err != nil {
		return err
	}
	if !file.Removable {
		return fmt.Errorf("%w: the main APT source file cannot be removed", ErrInvalid)
	}
	current, _, err := r.read(file)
	if err != nil {
		return err
	}
	if current.Revision != revision {
		return ErrRepositoryConflict
	}
	return os.Remove(filepath.Join(r.Root, file.Path))
}
