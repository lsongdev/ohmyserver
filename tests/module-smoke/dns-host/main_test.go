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

	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsDNSReleaseAndServesModuleResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dns.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	repository := dnsweb.NewRepository(db)
	if err := repository.InitializeModule(ctx); err != nil {
		t.Fatal(err)
	}
	service := dnsweb.NewLocalService(ctx, repository, t.TempDir())
	t.Cleanup(func() { _ = service.Close() })

	mux := http.NewServeMux()
	mux.Handle("/v1/dns/", http.StripPrefix("/v1/dns", dnsweb.New(repository, service, t.TempDir()).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/dns/manifest.json", `"id": "dns"`},
		{"/v1/dns/assets/module.js", "defineModule"},
		{"/v1/dns/zones", `"data":[]`},
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
