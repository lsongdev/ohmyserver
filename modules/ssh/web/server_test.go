package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiService struct {
	Service
	input  Input
	tested bool
	item   Server
	err    error
	ctx    context.Context
}

func (s *apiService) ListSSHProfiles(ctx context.Context) ([]Server, error) {
	s.ctx = ctx
	return []Server{s.item}, s.err
}
func (s *apiService) GetSSHProfile(ctx context.Context, _ int64) (Server, error) {
	s.ctx = ctx
	return s.item, s.err
}
func (s *apiService) SaveSSHProfile(ctx context.Context, _ int64, input Input) (Server, error) {
	s.ctx = ctx
	s.input = input
	return s.item, s.err
}
func (s *apiService) DeleteSSHProfile(ctx context.Context, _ int64) error {
	s.ctx = ctx
	return nil
}
func (s *apiService) TestSSHProfile(_ context.Context, input Input) error {
	s.tested, s.input = true, input
	return s.err
}
func (*apiService) SSHStats(context.Context, int64) (Stats, error)     { return Stats{}, nil }
func (*apiService) SSHMetrics(context.Context, int64) (Metrics, error) { return Metrics{}, nil }

func TestSSHProfileAPIKeepsCredentialsWriteOnly(t *testing.T) {
	service := &apiService{item: Server{ID: 4, Name: "nas", Host: "nas.local", Port: 22, Username: "admin", AuthType: "password", HasCredential: true}}
	handler := New(service).Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		handler.ServeHTTP(w, req)
		return w
	}
	created := request(http.MethodPost, "/servers", `{"name":"nas","host":"nas.local","username":"admin","auth_type":"password","password":"top-secret"}`)
	if created.Code != http.StatusCreated || service.input.Port != 22 {
		t.Fatalf("create: status=%d body=%s input=%+v", created.Code, created.Body.String(), service.input)
	}
	for _, secret := range []string{"top-secret", `"private_key":`, `"password":`} {
		if strings.Contains(created.Body.String(), secret) {
			t.Fatalf("response leaked credential field/value %q: %s", secret, created.Body.String())
		}
	}
	if response := request(http.MethodGet, "/servers/4", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"has_credential":true`) {
		t.Fatalf("detail: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request(http.MethodPost, "/servers/test", `{"name":"","host":"host","username":"root","auth_type":"password","password":"x"}`); response.Code != http.StatusBadRequest || service.tested {
		t.Fatalf("invalid connection test: status=%d body=%s called=%v", response.Code, response.Body.String(), service.tested)
	}
	if response := request(http.MethodPost, "/servers/test", `{"name":"test","host":"host","username":"root","auth_type":"password","password":"x","unexpected":true}`); response.Code != http.StatusBadRequest || service.tested {
		t.Fatalf("unknown field accepted: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request(http.MethodPost, "/servers/test", `{"name":"test","host":"host","username":"root","auth_type":"password","password":"x"}`); response.Code != http.StatusOK || !service.tested || service.input.Port != 22 {
		t.Fatalf("connection test: status=%d body=%s called=%v input=%+v", response.Code, response.Body.String(), service.tested, service.input)
	}
}

func TestSSHServiceErrorsMapToModuleProblem(t *testing.T) {
	service := &apiService{err: ErrConnection}
	handler := New(service).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/servers", nil))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "ssh_connection_failed") {
		t.Fatalf("connection error: status=%d body=%s", response.Code, response.Body.String())
	}
	service.err = errors.New("unexpected")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/servers/nope", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid id: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSSHProfileAPIPropagatesRequestContext(t *testing.T) {
	type contextKey struct{}
	service := &apiService{}
	request := httptest.NewRequest(http.MethodGet, "/servers", nil)
	request = request.WithContext(context.WithValue(request.Context(), contextKey{}, "trace"))
	response := httptest.NewRecorder()
	New(service).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.ctx == nil || service.ctx.Value(contextKey{}) != "trace" {
		t.Fatalf("status=%d context value=%v body=%s", response.Code, service.ctx, response.Body.String())
	}
}
