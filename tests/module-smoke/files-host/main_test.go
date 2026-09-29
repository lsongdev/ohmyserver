package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	files "github.com/lsongdev/ohmyserver/modules/files"
	filesweb "github.com/lsongdev/ohmyserver/modules/files/web"
)

func TestExternalHostLoadsFilesReleaseAndServesResources(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	service, err := files.NewLocalFilesystemAt(root)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	mux := http.NewServeMux()
	mux.Handle("/v1/files/", http.StripPrefix("/v1/files", filesweb.New(service).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/files/manifest.json", `"id": "files"`},
		{"/v1/files/assets/module.js", "createAPI"},
		{"/v1/files/entries", `"hello.txt"`},
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
