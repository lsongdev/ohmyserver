package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFilesystemListCreateUploadAndDelete(t *testing.T) {
	fs := NewLocalFilesystem()
	root := t.TempDir()
	ctx := context.Background()
	if err := fs.CreateFolder(ctx, root, "docs"); err != nil {
		t.Fatal(err)
	}
	if err := fs.CreateFile(ctx, filepath.Join(root, "docs"), "readme.txt", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Upload(ctx, root, "upload.bin", strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	listing, err := fs.List(ctx, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 2 || !listing.Entries[0].IsDir {
		t.Fatalf("entries=%#v", listing.Entries)
	}
	file, info, err := fs.Open(ctx, filepath.Join(root, "upload.bin"))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if info.Size() != 4 {
		t.Fatalf("size=%d", info.Size())
	}
	if err := fs.Delete(ctx, filepath.Join(root, "docs")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs")); !os.IsNotExist(err) {
		t.Fatalf("deleted path still exists: %v", err)
	}
}

func TestLocalFilesystemRejectsUnsafeNamesAndRootDeletion(t *testing.T) {
	fs := NewLocalFilesystem()
	root := t.TempDir()
	ctx := context.Background()
	if err := fs.CreateFile(ctx, root, "../escape", "bad"); err != ErrInvalidName {
		t.Fatalf("traversal error=%v", err)
	}
	if err := fs.Delete(ctx, ""); err != ErrInvalidPath {
		t.Fatalf("empty delete error=%v", err)
	}
	if err := fs.Delete(ctx, string(filepath.Separator)); err != ErrInvalidPath {
		t.Fatalf("root delete error=%v", err)
	}
}

func TestLocalFilesystemRootCapabilityRejectsTraversalAndSymlinkEscape(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	fs, err := NewLocalFilesystemAt(allowed)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	ctx := context.Background()
	if err := fs.Delete(ctx, allowed); err != ErrInvalidPath {
		t.Fatalf("delete capability root error=%v", err)
	}
	if _, err := fs.List(ctx, outside, false); err != ErrInvalidPath {
		t.Fatalf("outside absolute path error=%v", err)
	}
	traversal, err := filepath.Rel(allowed, outside)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.List(ctx, traversal, false); err != ErrInvalidPath {
		t.Fatalf("traversal path error=%v", err)
	}

	link := filepath.Join(allowed, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := fs.List(ctx, link, false); err != ErrInvalidPath {
		t.Fatalf("symlink listing error=%v", err)
	}
	if _, _, err := fs.Open(ctx, filepath.Join(link, "secret.txt")); err != ErrInvalidPath {
		t.Fatalf("symlink file open error=%v", err)
	}
	if err := fs.Delete(ctx, link); err != nil {
		t.Fatalf("delete symlink itself: %v", err)
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("deleting symlink removed outside target: %v", err)
	}
}

func TestNewLocalFilesystemAtRequiresExistingDirectory(t *testing.T) {
	if _, err := NewLocalFilesystemAt(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing root error=%v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalFilesystemAt(file); err != ErrInvalidPath {
		t.Fatalf("file root error=%v", err)
	}
}

func TestLocalFilesystemStopsBeforeIOWhenRequestIsCancelled(t *testing.T) {
	root := t.TempDir()
	fs := NewLocalFilesystem()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fs.List(ctx, root, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("list error=%v", err)
	}
	if err := fs.CreateFile(ctx, root, "cancelled.txt", "data"); !errors.Is(err, context.Canceled) {
		t.Fatalf("create error=%v", err)
	}
	if err := fs.Upload(ctx, root, "cancelled.bin", strings.NewReader("data")); !errors.Is(err, context.Canceled) {
		t.Fatalf("upload error=%v", err)
	}
	if err := fs.Delete(ctx, filepath.Join(root, "target")); !errors.Is(err, context.Canceled) {
		t.Fatalf("delete error=%v", err)
	}
	if _, _, err := fs.Open(ctx, filepath.Join(root, "target")); !errors.Is(err, context.Canceled) {
		t.Fatalf("open error=%v", err)
	}
	for _, name := range []string{"cancelled.txt", "cancelled.bin"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("cancelled request created %s: %v", name, err)
		}
	}
}

type cancelAfterFirstRead struct{ cancel context.CancelFunc }

func (r cancelAfterFirstRead) Read(buffer []byte) (int, error) {
	n := copy(buffer, "data")
	r.cancel()
	return n, nil
}

func TestContextReaderStopsBetweenChunks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := contextReader{ctx: ctx, reader: cancelAfterFirstRead{cancel: cancel}}
	buffer := make([]byte, 4)
	if n, err := reader.Read(buffer); n != 4 || err != nil {
		t.Fatalf("first read=(%d, %v)", n, err)
	}
	if n, err := reader.Read(buffer); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read=(%d, %v)", n, err)
	}
}
