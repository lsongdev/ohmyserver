package web

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	files "github.com/lsongdev/ohmyserver/modules/files"
)

func TestFilesAPIUploadListAndRangeDownload(t *testing.T) {
	root := t.TempDir()
	handler := New(files.NewLocalFilesystem()).Handler()

	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	_ = multipartWriter.WriteField("path", root)
	part, err := multipartWriter.CreateFormFile("file", "report.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "hello world")
	if err := multipartWriter.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/entries?path="+root, nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"report.txt"`) {
		t.Fatalf("list status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/download?path="+filepath.Join(root, "report.txt"), nil)
	request.Header.Set("Range", "bytes=6-10")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "world" {
		t.Fatalf("range download status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "report.txt") {
		t.Fatalf("Content-Disposition=%q", got)
	}
}

func TestFilesAPIErrorsAndUploadLimit(t *testing.T) {
	handler := New(files.NewLocalFilesystem()).Handler()
	t.Run("invalid delete path is json", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/entries?path=", nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("status=%d content-type=%q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
		}
	})
	t.Run("missing download maps to 404", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/download?path="+missing, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("oversized upload rejected", func(t *testing.T) {
		payload := strings.Repeat("x", int(maxUploadBytes)+1)
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("path", t.TempDir())
		part, err := writer.CreateFormFile("file", "large.bin")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(part, payload)
		_ = writer.Close()
		request := httptest.NewRequest(http.MethodPost, "/upload", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestHandlerServesModuleManifestAndEntry(t *testing.T) {
	handler := New(files.NewLocalFilesystem()).Handler()
	for _, path := range []string{"/manifest.json", "/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestStandaloneHandlerServesUIAndMountedAPI(t *testing.T) {
	root := t.TempDir()
	service, err := files.NewLocalFilesystemAt(root)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	handler := New(service).StandaloneHandler()
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "index.js") {
		t.Fatalf("GET / status=%d body=%q", page.Code, page.Body.String())
	}
	api := httptest.NewRecorder()
	handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/v1/files/entries?path="+url.QueryEscape(root), nil))
	if api.Code != http.StatusOK || !strings.Contains(api.Body.String(), `"data"`) {
		t.Fatalf("GET mounted API status=%d body=%q", api.Code, api.Body.String())
	}
}
