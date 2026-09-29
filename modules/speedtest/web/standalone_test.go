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
	h := New(standaloneService{}).Handler()
	for _, tc := range []struct{ path, contains string }{{"/", "assets/index.js"}, {"/manifest.json", "assets/module.js"}, {"/assets/module.js", "createAPI"}, {"/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/results", `"data":[]`}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}
