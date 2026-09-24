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

	acme "github.com/lsongdev/ohmyserver/modules/acme"
	acmeweb "github.com/lsongdev/ohmyserver/modules/acme/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsACMEReleaseAndServesModuleResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "acme.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	ctx := context.Background()
	repository := acmeweb.NewRepository(db)
	if err := repository.InitializeModule(ctx); err != nil {
		t.Fatal(err)
	}
	engine := acme.NewEngine(ctx, repository, nil, nil)
	defer engine.Close()

	mux := http.NewServeMux()
	mux.Handle("/v1/acme/", http.StripPrefix("/v1/acme", acmeweb.New(engine).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/acme/manifest.json", `"id": "acme"`},
		{"/v1/acme/assets/module.js", "defineModule"},
		{"/v1/acme/certificates", `"data":[]`},
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
