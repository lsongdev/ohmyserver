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

	widgetsweb "github.com/lsongdev/ohmyserver/modules/widgets/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsWidgetsReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "widgets.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	repository := widgetsweb.NewRepository(db)
	if err := repository.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/widgets/", http.StripPrefix("/v1/widgets", widgetsweb.New(repository).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/widgets/manifest.json", `"id": "widgets"`},
		{"/v1/widgets/assets/module.js", "defineModule"},
		{"/v1/widgets/widgets", `"data":[]`},
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
