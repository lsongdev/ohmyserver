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

	ipmiweb "github.com/lsongdev/ohmyserver/modules/ipmi/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsIPMIReleaseAndServesModuleResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ipmi.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	module := ipmiweb.NewLocalModule(db)
	if err := module.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/ipmi/", http.StripPrefix("/v1/ipmi", module.Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/ipmi/manifest.json", `"id": "ipmi"`},
		{"/v1/ipmi/assets/module.js", "createAPI"},
		{"/v1/ipmi/servers", `"data":[]`},
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
