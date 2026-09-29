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

	settingsweb "github.com/lsongdev/ohmyserver/modules/settings/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsSettingsReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	repository := settingsweb.NewRepository(db)
	if err := repository.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/settings/", http.StripPrefix("/v1/settings", settingsweb.New(repository).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/settings/manifest.json", `"id": "settings"`},
		{"/v1/settings/assets/module.js", "createAPI"},
		{"/v1/settings/entries", `"data":[]`},
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
