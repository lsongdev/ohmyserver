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

	clashweb "github.com/lsongdev/ohmyserver/modules/clash/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsClashReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "clash.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	module, err := clashweb.NewStandaloneLocalModule(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/clash/", http.StripPrefix("/v1/clash", module.Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/clash/manifest.json", `"id": "clash"`},
		{"/v1/clash/assets/module.js", "id: 'clash'"},
		{"/v1/clash/servers", `"data":[]`},
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
