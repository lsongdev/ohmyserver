package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiService struct {
	Service
	created   Input
	enabled   bool
	enabledID int64
	runID     int64
	execLimit int
}

func (s *apiService) CreateCronJobAPI(_ context.Context, input Input) (Job, error) {
	s.created = input
	return Job{ID: 12, Name: input.Name, Type: input.Type, Schedule: input.Schedule, Command: input.Command}, nil
}
func (s *apiService) SetCronJobEnabled(_ context.Context, id int64, enabled bool) error {
	s.enabledID, s.enabled = id, enabled
	return nil
}
func (s *apiService) RunCronJobNow(_ context.Context, id int64) error { s.runID = id; return nil }
func (s *apiService) CronJobExecutions(_ context.Context, _ int64, limit int) ([]Execution, error) {
	s.execLimit = limit
	return []Execution{}, nil
}

func TestCronModuleValidatesAndDelegatesActions(t *testing.T) {
	service := &apiService{}
	handler := New(service).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		handler.ServeHTTP(w, req)
		return w
	}
	if response := call(http.MethodPost, "/jobs", `{"name":"bad","type":"daily","schedule":"* * * * *","command":"echo"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid job: status=%d body=%s", response.Code, response.Body.String())
	}
	created := call(http.MethodPost, "/jobs", `{"name":"backup","type":"cron","schedule":"0 1 * * *","command":"backup","timeout":30}`)
	if created.Code != http.StatusCreated || service.created.Name != "backup" || service.created.Timeout != 30 {
		t.Fatalf("create: status=%d body=%s input=%+v", created.Code, created.Body.String(), service.created)
	}
	if response := call(http.MethodPost, "/jobs/12/enabled", `{"enabled":false}`); response.Code != http.StatusOK || service.enabledID != 12 || service.enabled {
		t.Fatalf("disable: status=%d body=%s id=%d enabled=%v", response.Code, response.Body.String(), service.enabledID, service.enabled)
	}
	if response := call(http.MethodPost, "/jobs/12/run", ""); response.Code != http.StatusAccepted || service.runID != 12 {
		t.Fatalf("run now: status=%d body=%s id=%d", response.Code, response.Body.String(), service.runID)
	}
	if response := call(http.MethodGet, "/jobs/12/executions", ""); response.Code != http.StatusOK || service.execLimit != 50 {
		t.Fatalf("executions: status=%d body=%s limit=%d", response.Code, response.Body.String(), service.execLimit)
	}
}

func TestCronRejectsInvalidActionIDsAndMissingEnabled(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/jobs/0/run", ""},
		{http.MethodPost, "/jobs/1/enabled", `{}`},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status=%d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestCronStandalonePageAndManifest(t *testing.T) {
	handler := New(&apiService{}).StandaloneHandler()
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `name="api-base" content="/v1/cron"`) {
		t.Fatalf("GET / status=%d body=%q", page.Code, page.Body.String())
	}
	for _, path := range []string{"/v1/cron/manifest.json", "/v1/cron/assets/index.js", "/v1/cron/assets/module.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("GET %s status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestCronWritesRequireJSONAndRejectUnknownFields(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, tc := range []struct{ contentType, body string }{
		{"text/plain", `{"name":"nightly","type":"cron","schedule":"0 1 * * *","command":"backup"}`},
		{"application/json", `{"name":"nightly","type":"cron","schedule":"0 1 * * *","command":"backup","secret":"x"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.contentType)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Errorf("Content-Type=%q status=%d body=%s", tc.contentType, response.Code, response.Body.String())
		}
	}
}
