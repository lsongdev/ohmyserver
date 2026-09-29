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
	input         Input
	action        Action
	actionCalls   int
	updatedConfig map[string]any
}

func (s *apiService) ListClash(context.Context) ([]DTO, error) { return []DTO{s.dto()}, nil }
func (s *apiService) CreateClash(_ context.Context, input Input) (DTO, error) {
	s.input = input
	return s.dto(), nil
}
func (s *apiService) GetClash(context.Context, int64) (DTO, error) { return s.dto(), nil }
func (s *apiService) UpdateClash(_ context.Context, _ int64, input Input) (DTO, error) {
	s.input = input
	return s.dto(), nil
}
func (*apiService) DeleteClash(context.Context, int64) error { return nil }
func (*apiService) TestClash(context.Context, int64) error   { return nil }
func (*apiService) ClashProxies(context.Context, int64) (any, error) {
	return map[string]any{"proxies": []any{}}, nil
}
func (*apiService) ClashRules(context.Context, int64) (any, error) {
	return map[string]any{"rules": []any{}}, nil
}
func (*apiService) ClashConnections(context.Context, int64) (any, error) {
	return map[string]any{"connections": []any{}}, nil
}
func (*apiService) ClashConfig(context.Context, int64) (any, error) {
	return map[string]any{"mode": "rule"}, nil
}
func (s *apiService) ClashAction(_ context.Context, _ int64, action Action) (any, error) {
	s.action, s.actionCalls = action, s.actionCalls+1
	return map[string]bool{"accepted": true}, nil
}
func (s *apiService) UpdateClashConfig(_ context.Context, _ int64, config map[string]any) (any, error) {
	s.updatedConfig = config
	return config, nil
}
func (*apiService) OpenClashStream(context.Context, int64) (Stream, error) { return nil, nil }
func (s *apiService) dto() DTO {
	return DTO{ID: 6, Name: "proxy", APIURL: "http://127.0.0.1:9090", HasSecret: s.input.Secret != nil}
}

func TestClashModuleRedactsSecretAndDelegatesConfig(t *testing.T) {
	service := &apiService{}
	handler := New(service).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		handler.ServeHTTP(w, request)
		return w
	}
	created := call(http.MethodPost, "/servers", `{"name":"proxy","api_url":"http://127.0.0.1:9090","secret":"super-secret"}`)
	if created.Code != http.StatusCreated || !service.dto().HasSecret || strings.Contains(created.Body.String(), "super-secret") {
		t.Fatalf("create: status=%d body=%s dto=%+v", created.Code, created.Body.String(), service.dto())
	}
	updated := call(http.MethodPut, "/servers/6", `{"name":"proxy","api_url":"http://127.0.0.1:9090"}`)
	if updated.Code != http.StatusOK || service.input.Secret != nil || strings.Contains(updated.Body.String(), "super-secret") {
		t.Fatalf("update without secret: status=%d body=%s input=%+v", updated.Code, updated.Body.String(), service.input)
	}
	config := call(http.MethodPatch, "/servers/6/config", `{"mode":"global","allow-lan":true}`)
	if config.Code != http.StatusOK || service.updatedConfig["mode"] != "global" {
		t.Fatalf("config update: status=%d body=%s config=%v", config.Code, config.Body.String(), service.updatedConfig)
	}
	action := call(http.MethodPost, "/servers/6/actions", `{"action":"select","selector":"Auto","proxy_name":"DIRECT"}`)
	if action.Code != http.StatusOK || service.actionCalls != 1 || service.action.ProxyName != "DIRECT" {
		t.Fatalf("action: status=%d body=%s action=%+v", action.Code, action.Body.String(), service.action)
	}
}

func TestClashModuleValidatesRequiredFieldsAndIDs(t *testing.T) {
	service := &apiService{}
	handler := New(service).Handler()
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/servers", `{"name":"","api_url":"http://localhost"}`},
		{http.MethodGet, "/servers/0", ""},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if response.Code != http.StatusBadRequest && response.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s %s: status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
}

func TestClashModuleRejectsNonJSONUnknownAndTrailingRequestBodies(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "wrong content type", contentType: "text/plain", body: `{"name":"proxy","api_url":"http://localhost"}`},
		{name: "unknown field", contentType: "application/json", body: `{"name":"proxy","api_url":"http://localhost","unexpected":true}`},
		{name: "multiple values", contentType: "application/json", body: `{"name":"proxy","api_url":"http://localhost"}{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", tc.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestHandlerServesModuleManifestAndEntry(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, path := range []string{"/manifest.json", "/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestHandlerServesUIAndMountedModuleAssets(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, path := range []string{"/", "/manifest.json", "/assets/index.js", "/assets/module.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, expected := range []string{
		`href="https://lsong.org/stylesheets/index.css"`,
		`src="assets/index.js"`,
		`name="api-base" content="."`,
	} {
		if !strings.Contains(page.Body.String(), expected) {
			t.Errorf("standalone index.html does not contain %q", expected)
		}
	}
}
