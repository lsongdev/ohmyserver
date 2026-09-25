package host

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/client"
	"github.com/lsongdev/ohmyserver/internal/app"
	dockermodule "github.com/lsongdev/ohmyserver/modules/docker"
	dockerweb "github.com/lsongdev/ohmyserver/modules/docker/web"
)

func TestDockerModuleRoundTripsThroughSDKAndIsolatedDaemon(t *testing.T) {
	type dockerRequest struct{ method, uri, body string }
	requests := make(chan dockerRequest, 32)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- dockerRequest{method: r.Method, uri: r.URL.RequestURI(), body: string(body)}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"Id":"container-1","Names":["/web"],"Image":"nginx:stable","State":"running","Status":"Up"}]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/container-1/start"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/container-1/json"):
			_, _ = w.Write([]byte(`{"Id":"container-1","Name":"/web","Created":"2024-01-01T00:00:00Z","Config":{"Image":"nginx:stable","Tty":true},"State":{"Status":"running"}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/container-1/logs"):
			_, _ = w.Write([]byte("hello from container\n"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/images/json"):
			_, _ = w.Write([]byte(`[{"Id":"sha256:image-1","RepoTags":["nginx:stable"],"Size":1024}]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
			_, _ = w.Write([]byte("{\"status\":\"Pulling from library/alpine\",\"progress\":\"50%\"}\n"))
		case r.Method == http.MethodPost && r.URL.Path == "/v1.41/build":
			_, _ = w.Write([]byte("{\"stream\":\"Step 1/1 : FROM scratch\\n\"}\n{\"stream\":\"Successfully built demo\\n\"}\n"))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1.41/containers/container-1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/images/sha256:image-1"):
			_, _ = w.Write([]byte(`[{"Untagged":"nginx:stable"}]`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/cache"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/networks/network-1"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes"):
			_, _ = w.Write([]byte(`{"Volumes":[{"Name":"cache","Driver":"local","Mountpoint":"/var/lib/docker/volumes/cache/_data"}],"Warnings":[]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/networks"):
			_, _ = w.Write([]byte(`[{"Id":"network-1","Name":"bridge","Driver":"bridge"}]`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/images/alpine/json"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such image: alpine"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/create"):
			_, _ = w.Write([]byte(`{"Id":"created-container","Warnings":[]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/volumes/create"):
			_, _ = w.Write([]byte(`{"Name":"cache","Driver":"local","Mountpoint":"/var/lib/docker/volumes/cache/_data"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/networks/create"):
			_, _ = w.Write([]byte(`{"Id":"network-created","Warning":""}`))
		default:
			http.Error(w, "unexpected Docker API request", http.StatusNotFound)
		}
	}))
	defer daemon.Close()

	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(daemon.URL),
		client.WithVersion("1.41"),
		client.WithHTTPClient(daemon.Client()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dockerClient.Close() })

	service := dockermodule.NewLocalService(context.Background(), &dockermodule.DockerClient{Client: dockerClient}, nil, t.TempDir(), t.TempDir())
	modules := app.NewModules()
	if err := modules.Register(dockerweb.New(service)); err != nil {
		t.Fatal(err)
	}
	handler := http.StripPrefix("/v1", modules.Handler())

	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/v1/docker/containers", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list containers: status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var payload struct {
		Data []struct {
			ID    string   `json:"id"`
			Names []string `json:"names"`
			Image string   `json:"image"`
			State string   `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode container list: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != "container-1" || payload.Data[0].Names[0] != "/web" || payload.Data[0].Image != "nginx:stable" || payload.Data[0].State != "running" {
		t.Fatalf("container DTO = %+v", payload.Data)
	}

	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, httptest.NewRequest(http.MethodPost, "/v1/docker/containers/container-1/start", nil))
	if startResponse.Code != http.StatusNoContent {
		t.Fatalf("start container: status=%d body=%s", startResponse.Code, startResponse.Body.String())
	}
	if response := call(http.MethodGet, "/v1/docker/containers/container-1", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"web"`) {
		t.Fatalf("inspect container: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/v1/docker/containers/container-1/logs", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "hello from container") {
		t.Fatalf("container logs: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/v1/docker/images", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"nginx:stable"`) {
		t.Fatalf("list images: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/v1/docker/images/pull", `{"name":"alpine"}`); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: progress") || !strings.Contains(response.Body.String(), "event: done") {
		t.Fatalf("pull image SSE: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/v1/docker/images/build", `{"name":"demo:latest","source_type":"dockerfile","dockerfile":"FROM scratch\\nLABEL purpose=integration"}`); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: log") || !strings.Contains(response.Body.String(), "event: done") {
		t.Fatalf("build image SSE: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/v1/docker/containers", `{"name":"demo","image":"alpine","command":"echo hello"}`); response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"created-container"`) {
		t.Fatalf("create container: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/v1/docker/volumes", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cache"`) {
		t.Fatalf("list volumes: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/v1/docker/volumes", `{"name":"cache","driver":"local"}`); response.Code != http.StatusCreated {
		t.Fatalf("create volume: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/v1/docker/networks", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"network-1"`) {
		t.Fatalf("list networks: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/v1/docker/networks", `{"name":"isolated","driver":"bridge","subnet":"10.44.0.0/24","gateway":"10.44.0.1","ip_range":"10.44.0.128/25"}`); response.Code != http.StatusCreated {
		t.Fatalf("create network: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, test := range []struct{ path, name string }{
		{"/v1/docker/containers/container-1", "container"},
		{"/v1/docker/images/sha256:image-1", "image"},
		{"/v1/docker/volumes/cache", "volume"},
		{"/v1/docker/networks/network-1", "network"},
	} {
		if response := call(http.MethodDelete, test.path, ""); response.Code != http.StatusNoContent {
			t.Fatalf("delete %s: status=%d body=%s", test.name, response.Code, response.Body.String())
		}
	}

	gotRequests := make(map[string]dockerRequest)
	for i := 0; i < 18; i++ {
		request := <-requests
		key := request.method + " " + strings.Split(request.uri, "?")[0]
		gotRequests[key] = request
	}
	for _, key := range []string{
		"GET /v1.41/containers/json",
		"POST /v1.41/containers/container-1/start",
		"GET /v1.41/containers/container-1/json",
		"GET /v1.41/containers/container-1/logs",
		"GET /v1.41/images/json",
		"POST /v1.41/images/create",
		"POST /v1.41/build",
		"GET /v1.41/images/alpine/json",
		"POST /v1.41/containers/create",
		"DELETE /v1.41/containers/container-1",
		"DELETE /v1.41/images/sha256:image-1",
		"GET /v1.41/volumes",
		"POST /v1.41/volumes/create",
		"DELETE /v1.41/volumes/cache",
		"GET /v1.41/networks",
		"POST /v1.41/networks/create",
		"DELETE /v1.41/networks/network-1",
	} {
		if _, ok := gotRequests[key]; !ok {
			t.Errorf("Docker daemon did not receive %s; got %#v", key, gotRequests)
		}
	}
	containerCreate := gotRequests["POST /v1.41/containers/create"]
	if !strings.Contains(containerCreate.uri, "name=demo") || !strings.Contains(containerCreate.body, `"Image":"alpine"`) || !strings.Contains(containerCreate.body, `"Cmd":["/bin/sh","-c","echo hello"]`) {
		t.Errorf("Docker container create request = %+v", containerCreate)
	}
	volumeCreate := gotRequests["POST /v1.41/volumes/create"]
	if !strings.Contains(volumeCreate.body, `"Name":"cache"`) || !strings.Contains(volumeCreate.body, `"Driver":"local"`) {
		t.Errorf("Docker volume create request = %+v", volumeCreate)
	}
	networkCreate := gotRequests["POST /v1.41/networks/create"]
	for _, value := range []string{`"Name":"isolated"`, `"Driver":"bridge"`, `"Subnet":"10.44.0.0/24"`, `"Gateway":"10.44.0.1"`, `"IPRange":"10.44.0.128/25"`} {
		if !strings.Contains(networkCreate.body, value) {
			t.Errorf("Docker network create payload missing %s: %+v", value, networkCreate)
		}
	}
	pullImage := gotRequests["POST /v1.41/images/create"]
	if !strings.Contains(pullImage.uri, "fromImage=alpine") || !strings.Contains(pullImage.uri, "tag=latest") {
		t.Errorf("Docker SDK image pull query = %+v", pullImage)
	}
	buildRequest := gotRequests["POST /v1.41/build"]
	if !strings.Contains(buildRequest.uri, "t=demo%3Alatest") && !strings.Contains(buildRequest.uri, "t=demo:latest") {
		t.Errorf("Docker build image tag missing from query: %+v", buildRequest)
	}
	archiveReader := tar.NewReader(bytes.NewReader([]byte(buildRequest.body)))
	header, err := archiveReader.Next()
	if err != nil {
		t.Fatalf("read Docker build context tar header: %v", err)
	}
	if header.Name != "Dockerfile" {
		t.Fatalf("first build context entry = %q, want Dockerfile", header.Name)
	}
	dockerfile, err := io.ReadAll(archiveReader)
	if err != nil {
		t.Fatalf("read Dockerfile from build context: %v", err)
	}
	if string(dockerfile) != "FROM scratch\\nLABEL purpose=integration" {
		t.Fatalf("Dockerfile context = %q", dockerfile)
	}
}
