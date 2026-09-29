package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLocalModuleComposesRepositoryRuntimeAndHandler(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dhcp.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	module := NewLocalModule(context.Background(), db, nil)
	if err := module.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	module.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /settings status=%d body=%s", response.Code, response.Body.String())
	}
	if err := module.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalModuleStartRestoresPersistedEnabledRuntime(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	ctx := context.Background()
	first := NewLocalModule(ctx, db, nil)
	if err := first.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := first.Repository().Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ListenAddr = "127.0.0.1:0"
	if err := first.Repository().UpdateSettings(ctx, *settings); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := NewLocalModule(ctx, db, nil)
	if err := second.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	status := second.Service().Runtime().Status()
	if !status.Running || status.ListenAddr == "" {
		t.Fatalf("persisted enabled runtime was not restored: %#v", status)
	}
}
