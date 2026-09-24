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
	h := New(emptyService{}).StandaloneHandler()
	for _, tc := range []struct{ path, contains string }{{"/", "/v1/ipmi/assets/index.js"}, {"/v1/ipmi/manifest.json", "assets/module.js"}, {"/v1/ipmi/assets/module.js", "defineModule"}, {"/v1/ipmi/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/v1/ipmi/servers", `"data":[]`}} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, r.Code, r.Body.String())
		}
	}
}
