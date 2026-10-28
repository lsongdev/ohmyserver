package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
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

// Every relative import the module loads must resolve to an embedded file;
// otherwise the panel fails at load time with only a console error. The
// check starts from the module's own scripts and follows their imports into
// node_modules, which holds whole packages, not only the files loaded.
func TestModuleImportsResolve(t *testing.T) {
	var queue []string
	err := fs.WalkDir(UI, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return fs.SkipDir
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs")) {
			queue = append(queue, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if seen[p] {
			continue
		}
		seen[p] = true
		data, err := fs.ReadFile(UI, p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		for _, m := range importRE.FindAllStringSubmatch(string(data), -1) {
			target := path.Join(path.Dir(p), m[1])
			if _, err := fs.Stat(UI, target); err != nil {
				t.Errorf("%s imports %s, which does not exist", p, m[1])
			} else if strings.HasSuffix(target, ".js") || strings.HasSuffix(target, ".mjs") {
				queue = append(queue, target)
			}
		}
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
