package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testService struct {
	info               Info
	infoErr, updateErr error
	check              bool
	queued             bool
}

func (s *testService) SystemInfo(_ context.Context, check bool) (Info, error) {
	s.check = check
	return s.info, s.infoErr
}
func (s *testService) QueueSystemUpdate(context.Context) error { s.queued = true; return s.updateErr }

func TestInfoAPI(t *testing.T) {
	service := &testService{info: Info{ProgramVersion: "v-test"}}
	response := httptest.NewRecorder()
	New(service).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/info?check=1", nil))
	if response.Code != http.StatusOK || !service.check {
		t.Fatalf("status=%d check=%v", response.Code, service.check)
	}
}
func TestUpdateAPIQueuesAndReportsErrors(t *testing.T) {
	service := &testService{}
	response := httptest.NewRecorder()
	New(service).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/update", nil))
	if response.Code != http.StatusAccepted || !service.queued {
		t.Fatalf("status=%d queued=%v", response.Code, service.queued)
	}
	service.updateErr = errors.New("unavailable")
	response = httptest.NewRecorder()
	New(service).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/update", nil))
	if response.Code != http.StatusInternalServerError {
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
