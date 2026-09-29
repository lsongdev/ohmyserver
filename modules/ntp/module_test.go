package ntp_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ntp "github.com/lsongdev/ohmyserver/modules/ntp"
	ntpweb "github.com/lsongdev/ohmyserver/modules/ntp/web"
	_ "modernc.org/sqlite"
)

type apiService struct {
	repo       *ntpweb.Repository
	lastAction string
	status     string
	fail       bool
	ctx        context.Context
}

func (s *apiService) UpdateNTPConfiguration(ctx context.Context, item ntp.Settings) error {
	s.ctx = ctx
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.fail {
		return errors.New("configuration rejected")
	}
	return s.repo.Update(ctx, item)
}
func (s *apiService) NTPRuntimeStatus(ctx context.Context) (any, error) {
	s.ctx = ctx
	return map[string]string{"state": s.status}, ctx.Err()
}
func (s *apiService) NTPAction(_ context.Context, action string) error {
	s.lastAction = action
	if s.fail {
		return errors.New("runtime unavailable")
	}
	s.status = action
	return nil
}

func newTestHandler(t *testing.T) (http.Handler, *apiService) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:ntp-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := ntpweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := &apiService{repo: repo, status: "stopped"}
	return ntpweb.New(repo, service).Handler(), service
}

func call(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestNTPSettingsAndActionAPI(t *testing.T) {
	handler, service := newTestHandler(t)
	settings := call(t, handler, http.MethodGet, "/settings", "")
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"listen_addr":":123"`) {
		t.Fatalf("settings: status=%d body=%s", settings.Code, settings.Body.String())
	}
	updated := call(t, handler, http.MethodPut, "/settings", `{"enabled":false,"listen_addr":":1123","stratum":4,"reference_id":"TEST"}`)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"reference_id":"TEST"`) {
		t.Fatalf("update: status=%d body=%s", updated.Code, updated.Body.String())
	}
	action := call(t, handler, http.MethodPost, "/action", `{"action":"reload"}`)
	if action.Code != http.StatusOK || service.lastAction != "reload" || !strings.Contains(action.Body.String(), `"state":"reload"`) {
		t.Fatalf("action: status=%d service=%+v body=%s", action.Code, service, action.Body.String())
	}
	service.fail = true
	failed := call(t, handler, http.MethodPost, "/action", `{"action":"stop"}`)
	if failed.Code != http.StatusBadRequest || !strings.Contains(failed.Body.String(), `"message":"runtime unavailable"`) {
		t.Fatalf("service failure: status=%d body=%s", failed.Code, failed.Body.String())
	}
}

func TestNTPConfigurationAPIPropagatesRequestContext(t *testing.T) {
	type contextKey struct{}
	handler, service := newTestHandler(t)
	request := httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader(`{"enabled":false,"listen_addr":":1123","stratum":4,"reference_id":"TEST"}`))
	request = request.WithContext(context.WithValue(request.Context(), contextKey{}, "request-trace"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.ctx == nil || service.ctx.Value(contextKey{}) != "request-trace" {
		t.Fatalf("status=%d service context=%v body=%s", response.Code, service.ctx, response.Body.String())
	}
}

func TestNTPConfigurationAPIHonorsCancelledRequest(t *testing.T) {
	handler, service := newTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader(`{"enabled":false,"listen_addr":":1123","stratum":4,"reference_id":"TEST"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if service.ctx == nil || !errors.Is(service.ctx.Err(), context.Canceled) {
		t.Fatalf("service did not receive cancelled request context: %v", service.ctx)
	}
	settings, err := service.repo.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.ListenAddr != ":123" {
		t.Fatalf("cancelled update was persisted: %+v (HTTP status %d)", settings, response.Code)
	}
}

func TestNTPStatusAPIPropagatesCancelledContext(t *testing.T) {
	handler, service := newTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code == http.StatusOK || service.ctx == nil || service.ctx.Err() != context.Canceled {
		t.Fatalf("status=%d service context=%v body=%s", response.Code, service.ctx, response.Body.String())
	}
}

func TestStandaloneRoutes(t *testing.T) {
	db, err := sql.Open("sqlite", "file:ntp-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := ntpweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := ntpweb.NewLocalService(repo)
	defer service.Close()
	handler := ntpweb.New(repo, service).Handler()
	for _, path := range []string{"/", "/manifest.json", "/assets/module.js", "/assets/index.js", "/settings", "/status"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}
