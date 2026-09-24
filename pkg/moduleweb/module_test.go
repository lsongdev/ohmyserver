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
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
