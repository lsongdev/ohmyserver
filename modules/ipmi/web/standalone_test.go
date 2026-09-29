package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type emptyService struct{ Service }

func (emptyService) ListIPMIServers(context.Context) ([]ServerDTO, error) { return []ServerDTO{}, nil }
func TestStandaloneServesPageManifestModuleAndAPI(t *testing.T) {
	h := New(emptyService{}).Handler()
	for _, tc := range []struct{ path, contains string }{{"/", "assets/index.js"}, {"/manifest.json", "assets/module.js"}, {"/assets/module.js", "createAPI"}, {"/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/servers", `"data":[]`}} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, r.Code, r.Body.String())
		}
	}
}
