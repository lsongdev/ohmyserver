package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncAppsRepositoryClonesAndUpdates(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, sourceDir, "init", "-b", "main")
	gitCommand(t, sourceDir, "config", "user.email", "test@example.com")
	gitCommand(t, sourceDir, "config", "user.name", "Test User")
	file := filepath.Join(sourceDir, "catalog.txt")
	if err := os.WriteFile(file, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, sourceDir, "add", "catalog.txt")
	gitCommand(t, sourceDir, "commit", "-m", "initial catalog")

	checkout := filepath.Join(t.TempDir(), "apps")
	if err := syncAppsRepository(t.Context(), sourceDir, checkout); err != nil {
		t.Fatalf("clone catalog: %v", err)
	}
	if err := os.WriteFile(file, []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, sourceDir, "add", "catalog.txt")
	gitCommand(t, sourceDir, "commit", "-m", "update catalog")
	if err := syncAppsRepository(t.Context(), sourceDir, checkout); err != nil {
		t.Fatalf("update catalog: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(checkout, "catalog.txt"))
	if err != nil || string(data) != "two" {
		t.Fatalf("catalog = %q, err = %v", data, err)
	}
}

func TestSyncAppsRepositoryRejectsNonGitDirectory(t *testing.T) {
	err := syncAppsRepository(t.Context(), "unused", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a git checkout") {
		t.Fatalf("error = %v, want non-git checkout error", err)
	}
}

func gitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
}
