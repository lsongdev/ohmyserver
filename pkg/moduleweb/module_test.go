package moduleweb

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWrapServesManifestAndAssetsAndKeepsAPIPaths(t *testing.T) {
	api := http.NewServeMux()
	api.HandleFunc("GET /items", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/items" {
			t.Errorf("API path = %q, want /items", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	assets := fstest.MapFS{
		"manifest.json": {Data: []byte(`{"id":"sample"}`)},
		"index.js":      {Data: []byte("export const ready = true;")},
	}
	handler := Wrap(api, assets)

	manifest := httptest.NewRecorder()
	handler.ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/manifest.json", nil))
	if manifest.Code != http.StatusOK || !strings.Contains(manifest.Body.String(), `"id":"sample"`) {
		t.Fatalf("manifest response: status=%d body=%s", manifest.Code, manifest.Body.String())
	}
	if manifest.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("manifest content type = %q", manifest.Header().Get("Content-Type"))
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/index.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "export const ready = true;" {
		t.Fatalf("asset response: status=%d body=%s", asset.Code, asset.Body.String())
	}
	if !strings.Contains(asset.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("asset content type = %q", asset.Header().Get("Content-Type"))
	}

	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/items", nil))
	if apiResponse.Code != http.StatusNoContent {
		t.Fatalf("API response status = %d", apiResponse.Code)
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", missing.Code)
	}
}

var _ fs.FS = fstest.MapFS{}

func TestSameHandlerAtRootAndArbitraryMount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"item_missing","message":"` + r.PathValue("id") + `"}}`))
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("stream lost Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: ready\n\n"))
		w.(http.Flusher).Flush()
	})
	mux.HandleFunc("GET /download", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0, 1, 255})
	})
	assets := fstest.MapFS{
		"index.html":    {Data: []byte(`<meta name="api-base" content="."><script type="module" src="assets/index.js"></script>`)},
		"manifest.json": {Data: []byte(`{"id":"demo"}`)},
	}
	module := Wrap(mux, assets)
	for _, base := range []string{"", "/v1/demo", "/panel/tools/proxy"} {
		t.Run(base, func(t *testing.T) {
			var handler http.Handler = module
			if base != "" {
				handler = http.StripPrefix(base, module)
			}
			for _, test := range []struct {
				method, path, body, contentType string
				status                          int
			}{
				{"GET", "/", `content="."`, "text/html", 200},
				{"GET", "/manifest.json", `"id":"demo"`, "application/json", 200},
				{"GET", "/assets/runtime/standalone.js", "mountStandalone", "javascript", 200},
				{"GET", "/assets/runtime/view.js", "createModuleView", "javascript", 200},
				{"GET", "/items/a%2Fb", `"code":"item_missing","message":"a/b"`, "application/json", 404},
				{"POST", "/items/a", `"code":"method_not_allowed"`, "application/json", 405},
				{"GET", "/unknown", `"code":"api_not_found"`, "application/json", 404},
				{"GET", "/assets/missing.js", "404", "text/plain", 404},
				{"GET", "/events", "data: ready\n\n", "text/event-stream", 200},
				{"GET", "/download", string([]byte{0, 1, 255}), "application/octet-stream", 200},
			} {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(test.method, base+test.path, nil))
				if w.Code != test.status || !strings.Contains(w.Body.String(), test.body) || !strings.Contains(w.Header().Get("Content-Type"), test.contentType) {
					t.Errorf("%s %s: %d %v %q", test.method, test.path, w.Code, w.Header(), w.Body.String())
				}
				if w.Code == 405 && w.Header().Get("Allow") != "GET, HEAD" {
					t.Errorf("lost Allow: %v", w.Header())
				}
			}
		})
	}
}
