package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type testModule struct {
	name    string
	handler http.Handler
}

func (m testModule) Name() string          { return m.name }
func (m testModule) Handler() http.Handler { return m.handler }

func TestModulesMountRelativeHandlers(t *testing.T) {
	modules := NewModules()
	if err := modules.Register(testModule{name: "demo", handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/items" {
			t.Errorf("module path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	modules.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/demo/items", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestModulesRejectDuplicateIDsAndReturnJSON404(t *testing.T) {
	modules := NewModules()
	module := testModule{name: "demo", handler: http.NotFoundHandler()}
	if err := modules.Register(module); err != nil {
		t.Fatal(err)
	}
	if err := modules.Register(module); err == nil {
		t.Fatal("duplicate module should fail")
	}
	response := httptest.NewRecorder()
	modules.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing/path", nil))
	if response.Code != http.StatusNotFound || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("status=%d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
}

func TestModuleErrorsAreJSONWithoutBreakingStreams(t *testing.T) {
	modules := NewModules()
	if err := modules.Register(testModule{name: "demo", handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: ok\n\n"))
			return
		}
		if r.URL.Path == "/method" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
	})}); err != nil {
		t.Fatal(err)
	}
	handler := modules.Handler()

	for _, tc := range []struct {
		path string
		code int
	}{
		{path: "/demo/missing", code: http.StatusNotFound},
		{path: "/demo/method", code: http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.code == http.StatusMethodNotAllowed {
			request.Method = http.MethodPost
		}
		handler.ServeHTTP(response, request)
		if response.Code != tc.code || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("%s: status=%d content-type=%q", tc.path, response.Code, response.Header().Get("Content-Type"))
		}
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/demo/stream", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" || response.Body.String() != "data: ok\n\n" {
		t.Fatalf("stream response changed: status=%d content-type=%q body=%q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}
