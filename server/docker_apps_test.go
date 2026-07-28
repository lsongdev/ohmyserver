package server

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/config"
)

func TestSyncAppsRepositoryClonesAndUpdates(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitForTest(t, sourceDir, "init", "-b", "main")
	gitForTest(t, sourceDir, "config", "user.email", "test@example.com")
	gitForTest(t, sourceDir, "config", "user.name", "Test User")

	catalogFile := filepath.Join(sourceDir, "catalog.txt")
	if err := os.WriteFile(catalogFile, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	gitForTest(t, sourceDir, "add", "catalog.txt")
	gitForTest(t, sourceDir, "commit", "-m", "initial catalog")

	checkoutDir := filepath.Join(t.TempDir(), "apps")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := syncAppsRepository(ctx, sourceDir, checkoutDir); err != nil {
		t.Fatalf("clone catalog: %v", err)
	}

	if err := os.WriteFile(catalogFile, []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	gitForTest(t, sourceDir, "add", "catalog.txt")
	gitForTest(t, sourceDir, "commit", "-m", "update catalog")

	if err := syncAppsRepository(ctx, sourceDir, checkoutDir); err != nil {
		t.Fatalf("update catalog: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(checkoutDir, "catalog.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "two" {
		t.Fatalf("catalog content = %q, want %q", got, "two")
	}
}

func TestSyncAppsRepositoryRejectsNonGitDirectory(t *testing.T) {
	appsDir := t.TempDir()
	err := syncAppsRepository(context.Background(), "unused", appsDir)
	if err == nil || !strings.Contains(err.Error(), "not a git checkout") {
		t.Fatalf("error = %v, want non-git checkout error", err)
	}
}

func TestLoadAppTemplatesReconcilesIncrementally(t *testing.T) {
	originalAppsDir := config.AppsDir
	config.AppsDir = t.TempDir()
	t.Cleanup(func() {
		config.AppsDir = originalAppsDir
	})

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	server := &Server{db: db}
	if err := server.initAppsSchema(); err != nil {
		t.Fatal(err)
	}

	writeTestApp(t, config.AppsDir, "first")
	if err := server.loadAppTemplates(); err != nil {
		t.Fatal(err)
	}
	if _, err := server.GetAppTemplate("first"); err != nil {
		t.Fatalf("first app was not loaded: %v", err)
	}

	changesBefore := sqliteTotalChanges(t, db)
	if err := server.loadAppTemplates(); err != nil {
		t.Fatal(err)
	}
	if changesAfter := sqliteTotalChanges(t, db); changesAfter != changesBefore {
		t.Fatalf("unchanged reload wrote %d rows, want 0", changesAfter-changesBefore)
	}

	updatedManifest := `{"name":"First updated","available":true}`
	if err := os.WriteFile(
		filepath.Join(config.AppsDir, "first", "manifest.json"),
		[]byte(updatedManifest),
		0644,
	); err != nil {
		t.Fatal(err)
	}
	changesBefore = sqliteTotalChanges(t, db)
	if err := server.loadAppTemplates(); err != nil {
		t.Fatal(err)
	}
	if changesAfter := sqliteTotalChanges(t, db); changesAfter-changesBefore != 1 {
		t.Fatalf("changed reload wrote %d rows, want 1", changesAfter-changesBefore)
	}
	app, err := server.GetAppTemplate("first")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "First updated" {
		t.Fatalf("updated app name = %q", app.Name)
	}

	if err := os.RemoveAll(filepath.Join(config.AppsDir, "first")); err != nil {
		t.Fatal(err)
	}
	writeTestApp(t, config.AppsDir, "second")
	if err := server.loadAppTemplates(); err != nil {
		t.Fatal(err)
	}

	if _, err := server.GetAppTemplate("first"); err != sql.ErrNoRows {
		t.Fatalf("removed app error = %v, want sql.ErrNoRows", err)
	}
	if _, err := server.GetAppTemplate("second"); err != nil {
		t.Fatalf("second app was not loaded: %v", err)
	}
}

func TestAppTemplatesPersistInMainDatabase(t *testing.T) {
	originalAppsDir := config.AppsDir
	config.AppsDir = filepath.Join(t.TempDir(), "apps")
	t.Cleanup(func() {
		config.AppsDir = originalAppsDir
	})
	writeTestApp(t, config.AppsDir, "persistent")

	databasePath := filepath.Join(t.TempDir(), "ohmyserver.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{db: db}
	if err := server.initAppsSchema(); err != nil {
		t.Fatal(err)
	}
	if err := server.loadAppTemplates(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedDB, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopenedDB.Close() })
	reopenedServer := &Server{db: reopenedDB}
	app, err := reopenedServer.GetAppTemplate("persistent")
	if err != nil {
		t.Fatalf("read persisted app: %v", err)
	}
	if app.Name != "persistent" {
		t.Fatalf("persisted app name = %q", app.Name)
	}
}

func sqliteTotalChanges(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var changes int64
	if err := db.QueryRow(`SELECT total_changes()`).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	return changes
}

func writeTestApp(t *testing.T, appsDir, id string) {
	t.Helper()
	appDir := filepath.Join(appsDir, id)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"` + id + `","available":true}`
	if err := os.WriteFile(filepath.Join(appDir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "docker-compose.yml"), []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}
}

func gitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
}
