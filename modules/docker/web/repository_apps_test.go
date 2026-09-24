package web

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	docker "github.com/lsongdev/ohmyserver/modules/docker"
	_ "modernc.org/sqlite"
)

func TestLoadDockerAppsReconcilesIncrementally(t *testing.T) {
	db := openCatalogTestDB(t)
	repo := NewRepository(db)
	if err := repo.InitializeAppCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	appsDir := t.TempDir()
	service := docker.NewLocalService(t.Context(), nil, repo, appsDir, t.TempDir())
	writeCatalogApp(t, appsDir, "first", `{"name":"First","available":true}`)
	if err := service.LoadDockerApps(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetAppTemplate(t.Context(), "first"); err != nil {
		t.Fatalf("first app was not loaded: %v", err)
	}

	before := catalogChanges(t, db)
	if err := service.LoadDockerApps(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after := catalogChanges(t, db); after != before {
		t.Fatalf("unchanged reload wrote %d rows, want 0", after-before)
	}

	writeCatalogApp(t, appsDir, "first", `{"name":"First updated","available":true}`)
	before = catalogChanges(t, db)
	if err := service.LoadDockerApps(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after := catalogChanges(t, db); after-before != 1 {
		t.Fatalf("changed reload wrote %d rows, want 1", after-before)
	}
	app, err := repo.GetAppTemplate(t.Context(), "first")
	if err != nil || app.Name != "First updated" {
		t.Fatalf("updated app = %#v, err = %v", app, err)
	}

	if err := os.RemoveAll(filepath.Join(appsDir, "first")); err != nil {
		t.Fatal(err)
	}
	writeCatalogApp(t, appsDir, "second", `{"name":"Second","available":true}`)
	if err := service.LoadDockerApps(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetAppTemplate(t.Context(), "first"); err != sql.ErrNoRows {
		t.Fatalf("removed app error = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.GetAppTemplate(t.Context(), "second"); err != nil {
		t.Fatalf("second app was not loaded: %v", err)
	}
}

func TestAppCatalogCanBeReadAfterReopeningDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	repo := NewRepository(db)
	if err := repo.InitializeAppCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	appsDir := t.TempDir()
	writeCatalogApp(t, appsDir, "persistent", `{"name":"persistent","available":true}`)
	service := docker.NewLocalService(t.Context(), nil, repo, appsDir, t.TempDir())
	if err := service.LoadDockerApps(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	app, err := NewRepository(reopened).GetAppTemplate(t.Context(), "persistent")
	if err != nil || app.Name != "persistent" {
		t.Fatalf("persisted app = %#v, err = %v", app, err)
	}
}

func openCatalogTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:docker-catalog-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func writeCatalogApp(t *testing.T, appsDir, id, manifest string) {
	t.Helper()
	dir := filepath.Join(appsDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}
}

func catalogChanges(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var changes int64
	if err := db.QueryRow(`SELECT total_changes()`).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	return changes
}
