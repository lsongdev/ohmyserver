package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	docker "github.com/lsongdev/ohmyserver/modules/docker"
	dockerweb "github.com/lsongdev/ohmyserver/modules/docker/web"
)

type testService struct{ docker.Service }

func (testService) ListDockerContainers(context.Context) ([]types.Container, error) {
	return []types.Container{}, nil
}

func TestStandaloneHostServesModuleManifestAssetsAndAPI(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/v1/docker/", http.StripPrefix("/v1/docker", dockerweb.New(testService{}).Handler()))
	host := httptest.NewServer(mux)
	defer host.Close()

	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/v1/docker/manifest.json", body: `"id": "docker"`},
		{path: "/v1/docker/assets/module.js", body: "createAPI"},
		{path: "/v1/docker/containers", body: `"data":[]`},
	} {
		t.Run(test.path, func(t *testing.T) {
			response, err := http.Get(host.URL + test.path)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || !strings.Contains(string(body), test.body) {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
}
