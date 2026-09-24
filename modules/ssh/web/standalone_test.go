package web

import (
	"context"
	sshmodule "github.com/lsongdev/ohmyserver/modules/ssh"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type emptyService struct{ Service }

func (emptyService) ListSSHProfiles(context.Context) ([]sshmodule.Server, error) {
	return []sshmodule.Server{}, nil
}
func TestStandaloneServesPageManifestModuleAndAPI(t *testing.T) {
	h := New(emptyService{}).StandaloneHandler()
	for _, tc := range []struct{ path, contains string }{{"/", "/v1/ssh/assets/index.js"}, {"/v1/ssh/manifest.json", "assets/module.js"}, {"/v1/ssh/assets/module.js", "defineModule"}, {"/v1/ssh/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/v1/ssh/servers", `"data":[]`}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}
