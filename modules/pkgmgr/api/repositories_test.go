package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/lsongdev/ohmyserver/modules/pkgmgr/system"
)

type repositoryManager struct {
	fakeManager
	name string
}

func (m *repositoryManager) Name() string { return m.name }

func TestRepositoryFiles(t *testing.T) {
	for _, tc := range []struct {
		manager, dir, id, content string
	}{
		{"apt", "etc/apt/sources.list.d", "vendor.sources", "Types: deb\nURIs: https://example.org/debian\nSuites: stable\nComponents: main\n"},
		{"dnf", "etc/yum.repos.d", "vendor.repo", "[vendor]\nbaseurl=https://example.org/rpm\nenabled=1\n"},
	} {
		t.Run(tc.manager, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, tc.dir)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			h := NewService(Options{Root: root, Manager: &repositoryManager{name: tc.manager}}).Handler()
			const base = "/repositories"
			path := base + "/" + tc.id
			request := func(method, path string, req repositoryRequest, want int) map[string]json.RawMessage {
				t.Helper()
				body, err := json.Marshal(req)
				if err != nil {
					t.Fatal(err)
				}
				got, out := call(t, h, method, path, string(body))
				if got != want {
					t.Fatalf("%s %s: got %d, want %d (%s)", method, path, got, want, out["error"])
				}
				return out
			}
			create := repositoryRequest{ID: tc.id, Content: tc.content}
			request(http.MethodPost, base, create, http.StatusNoContent)
			request(http.MethodPost, base, create, http.StatusConflict)
			request(http.MethodPost, base, repositoryRequest{ID: "../" + tc.id}, http.StatusBadRequest)
			out := request(http.MethodGet, base, repositoryRequest{}, http.StatusOK)
			var files []system.RepositoryFile
			if err := json.Unmarshal(out["data"], &files); err != nil || len(files) != 1 || files[0].ID != tc.id || files[0].Content != "" {
				t.Fatalf("list: %+v, %v", files, err)
			}
			read := func() system.RepositoryFile {
				t.Helper()
				out := request(http.MethodGet, path, repositoryRequest{}, http.StatusOK)
				var file system.RepositoryFile
				if err := json.Unmarshal(out["data"], &file); err != nil {
					t.Fatal(err)
				}
				return file
			}
			file := read()
			if file.Content != tc.content || file.Revision == "" {
				t.Fatalf("file: %+v", file)
			}
			update := repositoryRequest{Content: tc.content + "# changed\n", Revision: file.Revision}
			request(http.MethodPut, path, update, http.StatusNoContent)
			request(http.MethodPut, path, update, http.StatusConflict)
			request(http.MethodDelete, path+"?revision="+file.Revision, repositoryRequest{}, http.StatusConflict)
			request(http.MethodDelete, path+"?revision="+read().Revision, repositoryRequest{}, http.StatusNoContent)
			request(http.MethodGet, path, repositoryRequest{}, http.StatusNotFound)

			// A source filename must not expose a symlink's contents.
			target := filepath.Join(root, "private")
			if err := os.WriteFile(target, []byte("private"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, tc.id)); err != nil {
				t.Fatal(err)
			}
			request(http.MethodPost, base, create, http.StatusConflict)
			request(http.MethodGet, path, repositoryRequest{}, http.StatusBadGateway)
		})
	}
}
