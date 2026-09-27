package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testService struct {
	info    Info
	infoErr error
}

func (s *testService) SystemInfo(_ context.Context) (Info, error) {
	return s.info, s.infoErr
}

func TestInfoAPI(t *testing.T) {
	service := &testService{info: Info{ProgramVersion: "v-test"}}
	response := httptest.NewRecorder()
	New(service).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/info", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestHandlerServesModuleManifestAndEntry(t *testing.T) {
	handler := New(&testService{}).Handler()
	for _, path := range []string{"/manifest.json", "/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestStandaloneServesPageManifestAssetsAndAPI(t *testing.T) {
	handler := New(&testService{info: Info{ProgramName: "Test", Credits: []Credit{}}}).StandaloneHandler()
	for _, test := range []struct{ path, contains string }{
		{"/", `name="api-base"`},
		{"/v1/system/manifest.json", `assets/module.js`},
		{"/v1/system/assets/module.js", `createAPI`},
		{"/v1/system/assets/index.js", `lsong.org/scripts/react/index.js`},
		{"/v1/system/info", `"program_name":"Test"`},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.contains) {
			t.Fatalf("GET %s: status=%d body=%q", test.path, response.Code, response.Body.String())
		}
	}
}
