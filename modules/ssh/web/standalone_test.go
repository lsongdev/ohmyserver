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
	h := New(emptyService{}).Handler()
	for _, tc := range []struct{ path, contains string }{{"/", "assets/index.js"}, {"/manifest.json", "assets/module.js"}, {"/assets/module.js", "createAPI"}, {"/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/servers", `"data":[]`}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}
