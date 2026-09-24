package docker

import (
	"archive/tar"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/client"
)

func TestDockerImageBuildDirectoryAndGitRepositorySources(t *testing.T) {
	type buildContext struct{ files map[string]string }
	contexts := make(chan buildContext, 2)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/build") {
			http.Error(w, "unexpected Docker API request", http.StatusNotFound)
			return
		}
		files := make(map[string]string)
		tr := tar.NewReader(r.Body)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			files[header.Name] = string(data)
		}
		contexts <- buildContext{files: files}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"stream":"Successfully built test-image\n"}`+"\n")
	}))
	defer daemon.Close()

	dockerClient, err := client.NewClientWithOpts(client.WithHost(daemon.URL), client.WithVersion("1.41"), client.WithHTTPClient(daemon.Client()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dockerClient.Close() })
	service := NewLocalService(t.Context(), &DockerClient{Client: dockerClient}, nil, "", "")

	directory := t.TempDir()
	writeBuildFixture(t, directory)
	repository := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(repository, 0755); err != nil {
		t.Fatal(err)
	}
	writeBuildFixture(t, repository)
	gitCommand(t, repository, "init", "-b", "main")
	gitCommand(t, repository, "config", "user.email", "build-test@example.com")
	gitCommand(t, repository, "config", "user.name", "Build Test")
	gitCommand(t, repository, "add", "Dockerfile", "payload.txt")
	gitCommand(t, repository, "commit", "-m", "add build context")

	for _, test := range []struct {
		name    string
		request BuildImageRequest
	}{
		{name: "directory", request: BuildImageRequest{Name: "directory:test", SourceType: "directory", BuildDir: directory}},
		{name: "git repository", request: BuildImageRequest{Name: "repository:test", SourceType: "repository", Repository: repository, Branch: "main"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs []string
			if err := service.BuildDockerImage(t.Context(), test.request, func(line string) error {
				logs = append(logs, line)
				return nil
			}); err != nil {
				t.Fatalf("build image: %v", err)
			}
			if !strings.Contains(strings.Join(logs, "\n"), "Successfully built") {
				t.Fatalf("build logs do not include successful daemon output: %q", logs)
			}
			select {
			case got := <-contexts:
				if got.files["Dockerfile"] != "FROM scratch\nLABEL purpose=build-test\n" || got.files["payload.txt"] != "context payload\n" {
					t.Errorf("build context files = %#v", got.files)
				}
				if _, exists := got.files[".git/config"]; exists {
					t.Error("Git metadata leaked into Docker build context")
				}
			case <-t.Context().Done():
				t.Fatal("Docker daemon did not receive build context")
			}
		})
	}
}

func writeBuildFixture(t *testing.T, directory string) {
	t.Helper()
	for name, content := range map[string]string{
		"Dockerfile":  "FROM scratch\nLABEL purpose=build-test\n",
		"payload.txt": "context payload\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
