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

func TestLocalModuleOwnsRuntimeAndStandaloneHandler(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "cron.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	module := NewLocalModule(context.Background(), db, nil)
	if err := module.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	module.StandaloneHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/cron/jobs", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /v1/cron/jobs status=%d body=%s", response.Code, response.Body.String())
	}
	if err := module.Close(); err != nil {
		t.Fatal(err)
	}
}
