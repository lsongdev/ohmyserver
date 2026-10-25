package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/lsongdev/ci-go/docker"
)

// The assets satisfy the panel module contract (version 1).
func TestManifestFollowsContract(t *testing.T) {
	data, err := fs.ReadFile(UI, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Contract int      `json:"contract"`
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Entry    string   `json:"entry"`
		Styles   []string `json:"styles"`
		Home     string   `json:"home"`
		Nav      []struct {
			Path, Title string
		} `json:"nav"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Contract != 1 || !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(m.ID) || m.Title == "" {
		t.Fatalf("manifest = %+v", m)
	}
	for _, name := range append([]string{m.Entry, "index.html"}, m.Styles...) {
		if _, err := fs.Stat(UI, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	entry, _ := fs.ReadFile(UI, m.Entry)
	if !regexp.MustCompile(`export (function|const) mount\b`).Match(entry) {
		t.Error("entry does not export mount")
	}
	// Every navigation entry and the home page have a route.
	paths := []string{m.Home}
	for _, n := range m.Nav {
		paths = append(paths, n.Path)
	}
	for _, p := range paths {
		if !strings.Contains(string(entry), `path: '`+p+`'`) {
			t.Errorf("no route for %s", p)
		}
	}
}

var importRE = regexp.MustCompile(`(?:from\s*|import\s*\(?\s*)["'](\.{1,2}/[^"']+)["']`)

// Every relative import in every module must resolve to an embedded file;
// otherwise the panel fails at load time with only a console error.
func TestModuleImportsResolve(t *testing.T) {
	err := fs.WalkDir(UI, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs")) {
			return err
		}
		data, err := fs.ReadFile(UI, p)
		if err != nil {
			return err
		}
		for _, m := range importRE.FindAllStringSubmatch(string(data), -1) {
			if _, err := fs.Stat(UI, path.Join(path.Dir(p), m[1])); err != nil {
				t.Errorf("%s imports %s, which does not exist", p, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStatic(t *testing.T) {
	h := Static(UI)
	get := func(path, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	index := get("/", "")
	if index.Code != 200 || !strings.Contains(index.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("index: %d %v", index.Code, index.Header())
	}
	if w := get("/", index.Header().Get("ETag")); w.Code != http.StatusNotModified {
		t.Fatalf("revalidation: %d", w.Code)
	}
	for _, path := range []string{"/missing.js", "/pages/", "/../web.go"} {
		if w := get(path, ""); w.Code != 404 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}

type noRuntime struct{}

func (noRuntime) ImageExists(context.Context, string) (bool, error) { return true, nil }
func (noRuntime) Pull(context.Context, string, io.Writer) error     { return nil }
func (noRuntime) Run(context.Context, docker.Container, io.Writer) (int, error) {
	return 0, nil
}
func (noRuntime) CreateVolume(context.Context, string, map[string]string) error { return nil }
func (noRuntime) RemoveVolume(context.Context, string) error                    { return nil }

// Standalone, the password guards everything but webhooks, which carry
// their own signatures.
func TestStandaloneLeavesWebhooksOpen(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ci.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ci, err := New(context.Background(), db, Options{Runtime: noRuntime{}})
	if err != nil {
		t.Fatal(err)
	}
	defer ci.Close()
	h := ci.Standalone("s3cret")
	for path, want := range map[string]int{"/api/repos": 401, "/": 401, "/api/hooks/1": 404} {
		method := "GET"
		if strings.HasPrefix(path, "/api/hooks/") {
			method = "POST"
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader("{}")))
		if w.Code != want {
			t.Errorf("%s %s: %d, want %d", method, path, w.Code, want)
		}
	}
	req := httptest.NewRequest("GET", "/api/repos", nil)
	req.SetBasicAuth("admin", "s3cret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("with the password: %d", w.Code)
	}
}
