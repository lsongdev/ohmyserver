package docker

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestTarDirectoryIncludesBuildContextAndSkipsGitMetadata(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"Dockerfile":      "FROM scratch\n",
		"nested/app.conf": "enabled=true\n",
		".git/config":     "secret metadata\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive, err := tarDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	found := map[string]bool{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		found[header.Name] = true
	}
	if !found["Dockerfile"] || !found["nested/app.conf"] {
		t.Fatalf("build context entries = %#v", found)
	}
	if found[".git"] || found[".git/config"] {
		t.Fatalf("Git metadata was packed: %#v", found)
	}
}
