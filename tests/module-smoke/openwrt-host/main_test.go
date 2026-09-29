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

	openwrtweb "github.com/lsongdev/ohmyserver/modules/openwrt/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsOpenWrtReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "openwrt.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	repo := openwrtweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	module := openwrtweb.NewLocalModule(repo)
	mux := http.NewServeMux()
	mux.Handle("/v1/openwrt/", http.StripPrefix("/v1/openwrt", module.Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/openwrt/manifest.json", `"id": "openwrt"`},
		{"/v1/openwrt/assets/module.js", "createAPI"},
		{"/v1/openwrt/routers", `"data":[]`},
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
