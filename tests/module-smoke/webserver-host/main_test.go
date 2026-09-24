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

	webserverweb "github.com/lsongdev/ohmyserver/modules/webserver/web"
	"github.com/lsongdev/ohmyserver/modules/webserver/web/repository"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsWebServerReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "webserver.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	ctx := context.Background()
	repo := repository.NewRepository(db)
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	runtime := webserverweb.NewRuntime(ctx, repo, nil, nil)
	defer runtime.Close()
	service := webserverweb.NewLocalService(repo, runtime, nil, nil)
	mux := http.NewServeMux()
	mux.Handle("/v1/webserver/", http.StripPrefix("/v1/webserver", webserverweb.New(service).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/webserver/manifest.json", `"id": "webserver"`},
		{"/v1/webserver/assets/module.js", "defineModule"},
		{"/v1/webserver/listeners", `"data":[]`},
		{"/v1/webserver/policies", `"data":[]`},
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
