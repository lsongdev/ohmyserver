package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSafeFS builds a root with a data volume at /data (simulated as another
// device), a home inside it, and a passwd naming that home.
func newSafeFS(t *testing.T) (*FS, string) {
	t.Helper()
	f, root := newFS(t)
	for _, dir := range []string{"etc", "data/lsong/Pictures", "data/shared", "mnt/usb/photos", "srv/cache/a"} {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
	}
	os.WriteFile(filepath.Join(root, "etc/passwd"), []byte("root:x:0:0:root:/root:/bin/bash\n"+
		"lsong:x:1000:1000::/data/lsong:/bin/bash\nnobody:x:65534:65534::/nonexistent:/usr/sbin/nologin\n"), 0o644)
	os.WriteFile(filepath.Join(root, "data/lsong/Pictures/a.jpg"), []byte("jpeg"), 0o644)
	os.WriteFile(filepath.Join(root, "data/shared/report.pdf"), []byte("pdf!!"), 0o644)
	os.WriteFile(filepath.Join(root, "srv/cache/a/x.bin"), []byte("123456"), 0o644)
	os.WriteFile(filepath.Join(root, "srv/cache/y.bin"), []byte("12"), 0o644)
	// /data and /mnt/usb are mount points: everything below them is on
	// another device.
	data, usb := filepath.Join(root, "data"), filepath.Join(root, "mnt", "usb")
	f.mountsOf = func() []string { return []string{"/", "/data", "/mnt/usb"} }
	f.devOf = func(real string) (uint64, error) {
		if _, err := os.Lstat(real); err != nil {
			return 0, err
		}
		switch {
		case real == data || strings.HasPrefix(real, data+"/"):
			return 2, nil
		case real == usb || strings.HasPrefix(real, usb+"/"):
			return 3, nil
		}
		return 1, nil
	}
	return f, root
}

func TestRefusesMountsHomesAndWhatContainsThem(t *testing.T) {
	f, _ := newSafeFS(t)
	for p, why := range map[string]string{
		"/data":                  "",
		"/data/lsong":            "home directory",
		"/mnt/usb":               "mount point",
		"/mnt":                   "mounted file system", // contains /mnt/usb
		"/srv/.ohmyserver-trash": "trash",
	} {
		_, err := f.Preview([]string{p})
		if !errors.Is(err, ErrProtected) || !strings.Contains(err.Error(), why) {
			t.Errorf("%s: %v", p, err)
		}
		if err := f.Move([]string{p}, "/srv"); !errors.Is(err, ErrProtected) {
			t.Errorf("move %s: %v", p, err)
		}
		if _, err := f.Rename(p, "renamed"); !errors.Is(err, ErrProtected) {
			t.Errorf("rename %s: %v", p, err)
		}
	}
	// What is inside a home or a volume can be deleted.
	if _, err := f.Preview([]string{"/data/lsong/Pictures", "/data/shared"}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteNeedsAMatchingPreview(t *testing.T) {
	f, root := newSafeFS(t)
	plan, err := f.Preview([]string{"/srv/cache"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files != 2 || plan.Dirs != 2 || plan.Bytes != 8 {
		t.Fatalf("plan %+v", plan)
	}
	// Something new appeared since the preview.
	os.WriteFile(filepath.Join(root, "srv/cache/z.bin"), []byte("z"), 0o644)
	if err := f.Delete([]string{"/srv/cache"}, *plan); !errors.Is(err, ErrChanged) {
		t.Fatalf("stale preview: %v", err)
	}
	if err := f.Delete([]string{"/srv/cache"}, Plan{}); !errors.Is(err, ErrChanged) {
		t.Fatalf("no preview: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "srv/cache")); err != nil {
		t.Fatal("deleted without a matching preview")
	}
}

func TestTrashRestoreAndPurge(t *testing.T) {
	f, root := newSafeFS(t)
	plan, _ := f.Preview([]string{"/data/shared", "/srv/cache"})
	if err := f.Delete([]string{"/data/shared", "/srv/cache"}, *plan); err != nil {
		t.Fatal(err)
	}
	// Each goes to the trash of its own file system: a rename, no copy.
	if _, err := os.Stat(filepath.Join(root, "data/shared")); !os.IsNotExist(err) {
		t.Fatal("still there")
	}
	dataTrash, _ := os.ReadDir(filepath.Join(root, "data", TrashDir))
	rootTrash, _ := os.ReadDir(filepath.Join(root, TrashDir))
	if len(dataTrash) != 2 || len(rootTrash) != 2 { // item + record each
		t.Fatalf("trash: data %d, root %d", len(dataTrash), len(rootTrash))
	}
	items, err := f.Trash()
	if err != nil {
		t.Fatal(err)
	}
	// Both trashes are listed, newest first.
	if len(items) != 2 || items[0].Original != "/srv/cache" || items[0].Name != "cache" || items[0].Files != 2 ||
		items[1].Original != "/data/shared" || !strings.HasPrefix(items[1].ID, "/data/"+TrashDir+"/") {
		t.Fatalf("items %+v", items)
	}
	if err := f.Restore([]string{items[1].ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.Restore([]string{items[0].ID}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "srv/cache/a/x.bin")); string(data) != "123456" {
		t.Fatalf("restored %q", data)
	}
	// Delete again, then purge for good.
	plan, _ = f.Preview([]string{"/srv/cache"})
	f.Delete([]string{"/srv/cache"}, *plan)
	items, _ = f.Trash()
	purge, err := f.PurgePreview([]string{items[0].ID})
	if err != nil || purge.Files != 2 {
		t.Fatalf("purge preview %+v %v", purge, err)
	}
	if err := f.Purge([]string{items[0].ID}, Plan{}); !errors.Is(err, ErrChanged) {
		t.Fatalf("purge without preview: %v", err)
	}
	if err := f.Purge([]string{items[0].ID}, *purge); err != nil {
		t.Fatal(err)
	}
	if items, _ := f.Trash(); len(items) != 0 {
		t.Fatalf("left %+v", items)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "data/shared/report.pdf")); string(data) != "pdf!!" {
		t.Fatalf("restored from the volume's trash: %q", data)
	}
	// Only items directly in a trash can be restored or purged.
	for _, id := range []string{"/srv", "/data/lsong", "/" + TrashDir} {
		if _, err := f.PurgePreview([]string{id}); !errors.Is(err, ErrInvalid) {
			t.Errorf("purge %s: %v", id, err)
		}
	}
}

func TestMoveAcrossFileSystemsRefusesNestedMounts(t *testing.T) {
	f, _ := newSafeFS(t)
	f.mountsOf = nil // without a mount table the tree is walked
	if _, err := f.scan("/mnt", filepath.Join(f.Root, "mnt")); !errors.Is(err, ErrProtected) {
		t.Fatalf("scan crossed a mount: %v", err)
	}
}
