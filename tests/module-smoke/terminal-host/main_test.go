package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	terminalweb "github.com/lsongdev/ohmyserver/modules/terminal/web"
)

func TestExternalHostLoadsTerminalReleaseAndServesResources(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/v1/terminal/", http.StripPrefix("/v1/terminal", terminalweb.New(nil).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct{ path, body string }{
		{"/v1/terminal/manifest.json", `"id": "terminal"`},
		{"/v1/terminal/assets/module.js", "createAPI"},
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
