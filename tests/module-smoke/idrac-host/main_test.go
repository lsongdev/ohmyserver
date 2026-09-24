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

	idracweb "github.com/lsongdev/ohmyserver/modules/idrac/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsIDRACReleaseAndServesModuleResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "idrac.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	module := idracweb.NewLocalModule(db)
	if err := module.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/idrac/", http.StripPrefix("/v1/idrac", module.Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/idrac/manifest.json", `"id": "idrac"`},
		{"/v1/idrac/assets/module.js", "defineModule"},
		{"/v1/idrac/servers", `"data":[]`},
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
