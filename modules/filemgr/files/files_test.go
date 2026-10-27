package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFS(t *testing.T) (*FS, string) {
	t.Helper()
	root := t.TempDir()
	f, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, "srv", "app", "logs"), 0o755)
	os.WriteFile(filepath.Join(root, "srv", "app", "config.yaml"), []byte("port: 80\n"), 0o640)
	os.WriteFile(filepath.Join(root, "srv", "app", "logs", "a.log"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(root, "srv", "blob.bin"), []byte{0, 1, 2}, 0o644)
	os.Symlink("app/config.yaml", filepath.Join(root, "srv", "current"))
	return f, root
}

func TestListAndStat(t *testing.T) {
	f, _ := newFS(t)
	entries, err := f.List("/srv")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name+":"+e.Type)
	}
	if strings.Join(got, " ") != "app:dir blob.bin:file current:link" {
		t.Fatalf("got %v", got)
	}
	if e := entries[2]; e.Target != "app/config.yaml" || e.TargetIs != "file" || e.Path != "/srv/current" {
		t.Fatalf("link %+v", e)
	}
	if _, err := f.List("/nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if e, err := f.Stat("/"); err != nil || e.Name != "/" || e.Type != "dir" {
		t.Fatalf("root %+v %v", e, err)
	}
}

func TestStaysUnderRoot(t *testing.T) {
	f, root := newFS(t)
	os.WriteFile(filepath.Join(filepath.Dir(root), "outside.txt"), []byte("secret"), 0o644)
	defer os.Remove(filepath.Join(filepath.Dir(root), "outside.txt"))
	for _, p := range []string{"/../outside.txt", "/srv/../../outside.txt"} {
		if _, err := f.Read(p); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s escaped: %v", p, err)
		}
	}
	for _, p := range []string{"", "relative", "/a\x00b"} {
		if _, err := f.List(p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", p, err)
		}
	}
}

func TestReadWrite(t *testing.T) {
	f, root := newFS(t)
	text, err := f.Read("/srv/app/config.yaml")
	if err != nil || text.Text != "port: 80\n" || text.Binary {
		t.Fatalf("read %+v %v", text, err)
	}
	if text, _ := f.Read("/srv/blob.bin"); !text.Binary || text.Text != "" {
		t.Fatalf("binary %+v", text)
	}
	if err := f.Write("/srv/app/config.yaml", "port: 8080\n", false); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(root, "srv/app/config.yaml"))
	data, _ := os.ReadFile(filepath.Join(root, "srv/app/config.yaml"))
	if string(data) != "port: 8080\n" || info.Mode().Perm() != 0o640 {
		t.Fatalf("written %q %v", data, info.Mode())
	}
	if err := f.Write("/srv/app/config.yaml", "x", true); !errors.Is(err, ErrExists) {
		t.Fatalf("create over existing: %v", err)
	}
	if err := f.Write("/srv/new.txt", "new", true); err != nil {
		t.Fatal(err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "srv", "*.part")); len(leftovers) > 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}
}

func TestUpload(t *testing.T) {
	f, root := newFS(t)
	if _, err := f.Upload("/srv", "up.txt", strings.NewReader("one"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Upload("/srv", "up.txt", strings.NewReader("two"), false); !errors.Is(err, ErrExists) {
		t.Fatalf("no overwrite: %v", err)
	}
	if e, err := f.Upload("/srv", "up.txt", strings.NewReader("three"), true); err != nil || e.Size != 5 {
		t.Fatalf("overwrite %+v %v", e, err)
	}
	if _, err := f.Upload("/srv", "app", strings.NewReader("x"), true); !errors.Is(err, ErrExists) {
		t.Fatalf("over a directory: %v", err)
	}
	for _, name := range []string{"../x", "a/b", "..", ""} {
		if _, err := f.Upload("/srv", name, strings.NewReader("x"), true); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: %v", name, err)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(root, "srv", "up.txt")); string(data) != "three" {
		t.Fatalf("content %q", data)
	}
}

func TestMoveCopyRenameDelete(t *testing.T) {
	f, root := newFS(t)
	if err := f.Mkdir("/srv/backup"); err != nil {
		t.Fatal(err)
	}
	if err := f.Copy([]string{"/srv/app"}, "/srv/backup"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "srv/backup/app/logs/a.log")); string(data) != "hello" {
		t.Fatalf("copied %q", data)
	}
	if info, _ := os.Stat(filepath.Join(root, "srv/backup/app/config.yaml")); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", info.Mode())
	}
	if err := f.Copy([]string{"/srv/app"}, "/srv/backup"); !errors.Is(err, ErrExists) {
		t.Fatalf("copy again: %v", err)
	}
	if err := f.Copy([]string{"/srv/app"}, "/srv/app/logs"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("into itself: %v", err)
	}
	if err := f.Move([]string{"/srv/blob.bin", "/srv/current"}, "/srv/backup"); err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(filepath.Join(root, "srv/backup/current")); target != "app/config.yaml" {
		t.Fatalf("link moved as %q", target)
	}
	if _, err := f.Rename("/srv/backup", "old"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Rename("/srv/old", "../x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad name: %v", err)
	}
	if err := f.Delete([]string{"/srv/old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "srv/old")); !os.IsNotExist(err) {
		t.Fatal("not deleted")
	}
	if err := f.Delete([]string{"/"}); !errors.Is(err, ErrProtected) {
		t.Fatalf("root: %v", err)
	}
}

func TestProtectsSystemDirectories(t *testing.T) {
	f := &FS{Root: "/"}
	for _, p := range []string{"/", "/etc", "/usr/", "/var"} {
		if err := f.Delete([]string{p}); !errors.Is(err, ErrProtected) {
			t.Errorf("delete %s: %v", p, err)
		}
		if err := f.Move([]string{p}, "/tmp"); !errors.Is(err, ErrProtected) {
			t.Errorf("move %s: %v", p, err)
		}
	}
}

func TestUnixMode(t *testing.T) {
	for mode, want := range map[os.FileMode]string{
		os.ModeDir | 0o755:                          "drwxr-xr-x",
		os.ModeSymlink | 0o777:                      "lrwxrwxrwx",
		0o4755&^0o4000 | os.ModeSetuid:              "-rwsr-xr-x",
		os.ModeDir | os.ModeSticky | 0o1777&^0o1000: "drwxrwxrwt",
		os.ModeDevice | os.ModeCharDevice | 0o620:   "crw--w----",
		0o644: "-rw-r--r--",
	} {
		if got := unixMode(mode); got != want {
			t.Errorf("%v: got %s, want %s", mode, got, want)
		}
	}
}
