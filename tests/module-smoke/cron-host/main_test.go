package main

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	cronweb "github.com/lsongdev/ohmyserver/modules/cron/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsCronReleaseAndServesModuleResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "cron.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	module := cronweb.NewLocalModule(context.Background(), db, nil)
	if err := module.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer module.Close()

	mux := http.NewServeMux()
	mux.Handle("/v1/cron/", http.StripPrefix("/v1/cron", module.Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/cron/manifest.json", `"id": "cron"`},
		{"/v1/cron/assets/module.js", "createAPI"},
		{"/v1/cron/jobs", `"data":[]`},
	} {
		response, err := http.Get(host.URL + test.path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), test.body) {
			t.Errorf("GET %s status=%d body=%s", test.path, response.StatusCode, body)
		}
	}
}
