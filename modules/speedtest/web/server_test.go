package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type contextKey string

type testService struct {
	Service
	results  []Result
	servers  []ServerInfo
	err      error
	gotType  string
	deleted  int64
	run      bool
	runValue any
}

func (s *testService) ListResults(context.Context) ([]Result, error) { return s.results, s.err }
func (s *testService) DeleteResult(_ context.Context, id int64) error {
	s.deleted = id
	return s.err
}
func (s *testService) ListServers(_ context.Context, kind string) ([]ServerInfo, error) {
	s.gotType = kind
	return s.servers, s.err
}
func (s *testService) RunSpeedtest(ctx context.Context, w http.ResponseWriter, _ *http.Request) {
	s.run = true
	s.runValue = ctx.Value(contextKey("request"))
	w.WriteHeader(http.StatusAccepted)
}

func TestSpeedtestAPIContracts(t *testing.T) {
	service := &testService{servers: []ServerInfo{{ID: "1", Name: "local"}}}
	handler := New(service).Handler()
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}

	if response := request(http.MethodGet, "/servers"); response.Code != http.StatusBadRequest {
		t.Fatalf("missing type: %d %s", response.Code, response.Body.String())
	}
	if service.gotType != "" {
		t.Fatalf("invalid server type reached service: %q", service.gotType)
	}
	if response := request(http.MethodGet, "/servers?type=librespeed"); response.Code != http.StatusOK || service.gotType != "librespeed" {
		t.Fatalf("servers: %d %s, type=%q", response.Code, response.Body.String(), service.gotType)
	}
	if response := request(http.MethodDelete, "/results/0"); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid result id: %d %s", response.Code, response.Body.String())
	}
	if response := request(http.MethodDelete, "/results/7"); response.Code != http.StatusNoContent || service.deleted != 7 {
		t.Fatalf("delete result: %d %s, id=%d", response.Code, response.Body.String(), service.deleted)
	}
	runRequest := httptest.NewRequest(http.MethodPost, "/run", nil).WithContext(context.WithValue(context.Background(), contextKey("request"), "propagated"))
	runResponse := httptest.NewRecorder()
	handler.ServeHTTP(runResponse, runRequest)
	if runResponse.Code != http.StatusAccepted || !service.run || service.runValue != "propagated" {
		t.Fatalf("run: %d %s, context=%v", runResponse.Code, runResponse.Body.String(), service.runValue)
	}
}

func TestSpeedtestServiceErrorsAreJSON(t *testing.T) {
	service := &testService{err: errors.New("service offline")}
	handler := New(service).Handler()
	for _, path := range []string{"/results", "/servers?type=speedtest"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code < 500 || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("%s: status=%d content-type=%q body=%s", path, response.Code, response.Header().Get("Content-Type"), response.Body.String())
		}
	}
}
