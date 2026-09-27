package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	projectsweb "github.com/lsongdev/ohmyserver/modules/projects/web"
	_ "modernc.org/sqlite"
)

func TestExternalHostLoadsProjectsReleaseAndServesResources(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	repository := projectsweb.NewRepository(db)
	if err := repository.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/projects/", http.StripPrefix("/v1/projects", projectsweb.New(repository, nil).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/projects/manifest.json", `"id": "projects"`},
		{"/v1/projects/assets/module.js", "defineModule"},
		{"/v1/projects/projects", `"data":[]`},
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

	created, err := http.Post(host.URL+"/v1/projects/projects", "application/json", strings.NewReader(`{"name":"release-smoke","repo":"https://example.com/team/app.git"}`))
	if err != nil {
		t.Fatal(err)
	}
	createdBody, readErr := io.ReadAll(created.Body)
	_ = created.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if created.StatusCode != http.StatusCreated || !strings.Contains(string(createdBody), `"webhook_secret_once"`) {
		t.Fatalf("POST /v1/projects/projects status=%d body=%s", created.StatusCode, createdBody)
	}
	listed, err := http.Get(host.URL + "/v1/projects/projects")
	if err != nil {
		t.Fatal(err)
	}
	listedBody, readErr := io.ReadAll(listed.Body)
	_ = listed.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if listed.StatusCode != http.StatusOK || !strings.Contains(string(listedBody), fmt.Sprintf(`"name":"release-smoke"`)) {
		t.Fatalf("GET /v1/projects/projects status=%d body=%s", listed.StatusCode, listedBody)
	}
}
