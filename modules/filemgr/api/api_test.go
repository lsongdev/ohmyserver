package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func newServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "www", "css"), 0o755)
	os.WriteFile(filepath.Join(root, "www", "index.html"), []byte("<h1>hi</h1>"), 0o644)
	os.WriteFile(filepath.Join(root, "www", "css", "site.css"), []byte("body{}"), 0o644)
	s, err := New(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, root
}

func do(t *testing.T, req *http.Request) (int, string, http.Header) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

func TestUploadAndDownload(t *testing.T) {
	srv, root := newServer(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, content := range map[string]string{"notes.txt": "hello world", "../evil.txt": "x"} {
		part, _ := mw.CreateFormFile("file", name)
		part.Write([]byte(content))
	}
	mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/upload?dir=/www", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if status, out, _ := do(t, req); status != 200 || !strings.Contains(out, `"path":"/www/notes.txt"`) {
		t.Fatalf("upload: %d %s", status, out)
	}
	// The path in a file name is dropped: it lands in /www, not above it.
	if _, err := os.Stat(filepath.Join(root, "www", "evil.txt")); err != nil {
		t.Fatalf("base name not used: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "evil.txt")); err == nil {
		t.Fatal("upload escaped its directory")
	}

	req, _ = http.NewRequest("GET", srv.URL+"/download?path=/www/notes.txt", nil)
	req.Header.Set("Range", "bytes=6-")
	status, out, h := do(t, req)
	if status != http.StatusPartialContent || out != "world" || !strings.Contains(h.Get("Content-Disposition"), `attachment; filename=notes.txt`) {
		t.Fatalf("range: %d %q %v", status, out, h)
	}
	req, _ = http.NewRequest("GET", srv.URL+"/download?path=/www/index.html&inline=1", nil)
	if _, _, h := do(t, req); !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("inline HTML is not sandboxed: %v", h)
	}
}

func TestDirectoryArchive(t *testing.T) {
	srv, _ := newServer(t)
	resp, err := http.Get(srv.URL + "/download?path=/www")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
	}
	sort.Strings(names)
	if strings.Join(names, " ") != "www/ www/css/ www/css/site.css www/index.html" {
		t.Fatalf("archive %v", names)
	}
}

func TestErrors(t *testing.T) {
	srv, _ := newServer(t)
	for _, c := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/list?path=/nope", "", 404},
		{"GET", "/list?path=relative", "", 400},
		{"POST", "/delete", `{"paths":["/"]}`, 403},
		{"POST", "/mkdir", `{"path":"/www"}`, 409},
		{"POST", "/copy", `{"paths":["/www"],"to":"/www/css"}`, 400},
		{"POST", "/rename", `{"path":"/www/index.html","name":"a/b"}`, 400},
		{"POST", "/move", `{"paths":["/www/index.html"],"to":"/www/css","extra":1}`, 400},
	} {
		req, _ := http.NewRequest(c.method, srv.URL+c.path, strings.NewReader(c.body))
		if status, out, _ := do(t, req); status != c.status {
			t.Errorf("%s %s: %d %s", c.method, c.path, status, out)
		}
	}
}
