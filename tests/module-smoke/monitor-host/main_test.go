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

	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
	monitorweb "github.com/lsongdev/ohmyserver/modules/monitor/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsMonitorReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	repository := monitorweb.NewRepository(db)
	if err := repository.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/monitor/", http.StripPrefix("/v1/monitor", monitorweb.New(repository, monitor.LocalCollector{}).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/monitor/manifest.json", `"id": "monitor"`},
		{"/v1/monitor/assets/module.js", "defineModule"},
		{"/v1/monitor/hosts", `"data":[]`},
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
