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

func TestLocalModuleComposesRepositoryServiceAndHandler(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ntp.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	module := NewLocalModule(db, nil)
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
