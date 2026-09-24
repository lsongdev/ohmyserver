package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type standaloneService struct{ Service }

func (standaloneService) ListResults(context.Context) ([]Result, error) { return []Result{}, nil }
func TestStandaloneServesPageManifestModuleAndAPI(t *testing.T) {
	h := New(standaloneService{}).StandaloneHandler()
	for _, tc := range []struct{ path, contains string }{{"/", "/v1/speedtest/assets/index.js"}, {"/v1/speedtest/manifest.json", "assets/module.js"}, {"/v1/speedtest/assets/module.js", "defineModule"}, {"/v1/speedtest/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/v1/speedtest/results", `"data":[]`}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}
