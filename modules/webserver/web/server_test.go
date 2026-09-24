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
	listener        Listener
	listenerDeleted int64
	route           Route
	routeHostID     int64
}

func (s *apiService) CreateWebListenerAPI(_ context.Context, item Listener) (Listener, error) {
	s.listener = item
	item.ID = 3
	return item, nil
}
func (s *apiService) DeleteWebListenerAPI(_ context.Context, id int64) error {
	s.listenerDeleted = id
	return nil
}
func (s *apiService) CreateWebRouteAPI(_ context.Context, hostID int64, item Route) (Route, error) {
	s.routeHostID, s.route = hostID, item
	item.ID = 5
	return item, nil
}

func TestWebserverModuleListenerAndNestedRouteContracts(t *testing.T) {
	service := &apiService{}
	handler := New(service).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if response := call(http.MethodPost, "/listeners", `{"name":"","address":"127.0.0.1:80","protocol":"http"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid listener: status=%d body=%s", response.Code, response.Body.String())
	}
	created := call(http.MethodPost, "/listeners", `{"name":"public","address":"0.0.0.0:80","protocol":"http","enabled":true}`)
	if created.Code != http.StatusCreated || service.listener.Name != "public" || !strings.Contains(created.Body.String(), `"id":3`) {
		t.Fatalf("create listener: status=%d body=%s item=%+v", created.Code, created.Body.String(), service.listener)
	}
	if response := call(http.MethodDelete, "/listeners/0", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid listener id: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodDelete, "/listeners/3", ""); response.Code != http.StatusNoContent || service.listenerDeleted != 3 {
		t.Fatalf("delete listener: status=%d body=%s id=%d", response.Code, response.Body.String(), service.listenerDeleted)
	}
	route := call(http.MethodPost, "/hosts/9/routes", `{"name":"api","path":"/api/","handler_type":"proxy","upstream_url":"http://127.0.0.1:8080"}`)
	if route.Code != http.StatusCreated || service.routeHostID != 9 || service.route.HostID != 9 || service.route.Name != "api" {
		t.Fatalf("create nested route: status=%d body=%s host-id=%d route=%+v", route.Code, route.Body.String(), service.routeHostID, service.route)
	}
}

func TestWebserverModuleRejectsMalformedRouteAndPathIDs(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/hosts/not-an-id/routes", `{"name":"api","path":"/","handler_type":"proxy"}`},
		{http.MethodPost, "/hosts/1/routes", `{"name":"api","path":"" ,"handler_type":"proxy"}`},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
}

func TestWebserverModuleServesManifestAndOwnedAssets(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, path := range []string{"/manifest.json", "/assets/index.js", "/assets/api.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestStandaloneHandlerServesPageAndHostScopedModuleAssets(t *testing.T) {
	handler := New(&apiService{}).StandaloneHandler()
	for _, path := range []string{"/", "/v1/webserver/manifest.json", "/v1/webserver/assets/module.js", "/v1/webserver/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}
