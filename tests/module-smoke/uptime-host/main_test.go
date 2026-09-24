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

	uptime "github.com/lsongdev/ohmyserver/modules/uptime"
	uptimeweb "github.com/lsongdev/ohmyserver/modules/uptime/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsUptimeReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "uptime.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	repository := uptimeweb.NewRepository(db)
	if err := repository.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	module := uptimeweb.New(repository, uptime.NewChecker(repository))
	mux := http.NewServeMux()
	mux.Handle("/v1/uptime/", http.StripPrefix("/v1/uptime", module.Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/uptime/manifest.json", `"id": "uptime"`},
		{"/v1/uptime/assets/module.js", "defineModule"},
		{"/v1/uptime/monitors", `"data":[]`},
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
