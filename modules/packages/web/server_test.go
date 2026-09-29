package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	packages "github.com/lsongdev/ohmyserver/modules/packages"
)

type apiService struct {
	packages.Service
	view, query, operation, name string
	err                          error
}

func (s *apiService) PackageCatalog(_ context.Context, view, query string) (packages.Catalog, error) {
	s.view, s.query = view, query
	return packages.Catalog{Manager: "apt", Packages: []packages.Package{}}, s.err
}
func (s *apiService) ChangePackages(_ context.Context, operation, name string) (packages.ChangeResult, error) {
	s.operation, s.name = operation, name
	return packages.ChangeResult{Operation: operation}, s.err
}

func TestPackageAPISelectionAndMutationContracts(t *testing.T) {
	service := &apiService{}
	handler := New(service).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if response := call(http.MethodGet, "/packages?q=nginx", ""); response.Code != http.StatusOK || service.view != "installed" || service.query != "nginx" {
		t.Fatalf("default catalog: status=%d body=%s view=%q query=%q", response.Code, response.Body.String(), service.view, service.query)
	}
	if response := call(http.MethodPost, "/install", `{"name":"  nginx  "}`); response.Code != http.StatusOK || service.operation != "install" || service.name != "nginx" {
		t.Fatalf("install: status=%d body=%s operation=%q name=%q", response.Code, response.Body.String(), service.operation, service.name)
	}
	if response := call(http.MethodPost, "/upgrade", ""); response.Code != http.StatusOK || service.operation != "upgrade" || service.name != "" {
		t.Fatalf("upgrade all: status=%d body=%s operation=%q name=%q", response.Code, response.Body.String(), service.operation, service.name)
	}
	if response := call(http.MethodPost, "/remove", `{"name":" "}`); response.Code != http.StatusBadRequest {
		t.Fatalf("empty package name: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPackageAPIMapsUnavailableService(t *testing.T) {
	service := &apiService{err: packages.ErrUnavailable}
	handler := New(service).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/packages", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "package_error") {
		t.Fatalf("unavailable: status=%d body=%s", response.Code, response.Body.String())
	}
	service.err = errors.New("unexpected")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/refresh", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("internal service error: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerServesPageAssetsAndMountedAPI(t *testing.T) {
	handler := New(&apiService{}).Handler()
	for _, path := range []string{"/", "/manifest.json", "/assets/module.js", "/assets/index.js", "/packages"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}
