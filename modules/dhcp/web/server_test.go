package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

type apiService struct {
	repo       *Repository
	lastAction string
	status     string
	fail       bool
	ctx        context.Context
}

func (s *apiService) UpdateDHCPConfiguration(ctx context.Context, item Settings) error {
	s.ctx = ctx
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.fail {
		return errors.New("configuration rejected")
	}
	return s.repo.UpdateSettings(ctx, item)
}
func (s *apiService) SaveReservation(ctx context.Context, id int64, item StaticLease) error {
	s.ctx = ctx
	return s.repo.SaveStaticLease(ctx, id, item)
}
func (s *apiService) DeleteReservation(ctx context.Context, id int64) error {
	s.ctx = ctx
	return s.repo.DeleteStaticLease(ctx, id)
}
func (s *apiService) DeleteDHCPLease(ctx context.Context, id int64) error {
	s.ctx = ctx
	return s.repo.DeleteLease(ctx, id)
}
func (s *apiService) DHCPRuntimeStatus(ctx context.Context) (any, error) {
	s.ctx = ctx
	return map[string]string{"state": s.status}, ctx.Err()
}
func (s *apiService) DHCPAction(_ context.Context, action string) error {
	s.lastAction = action
	if s.fail {
		return errors.New("runtime unavailable")
	}
	s.status = action
	return nil
}

func newTestHandler(t *testing.T) (http.Handler, *apiService) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dhcp-test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := &apiService{repo: repo, status: "stopped"}
	return New(repo, service).Handler(), service
}

func call(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestDHCPSettingsAndStaticLeaseAPI(t *testing.T) {
	handler, _ := newTestHandler(t)
	settings := call(t, handler, http.MethodGet, "/settings", "")
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"listen_addr":":67"`) {
		t.Fatalf("settings: status=%d body=%s", settings.Code, settings.Body.String())
	}
	updated := call(t, handler, http.MethodPut, "/settings", `{"enabled":false,"listen_addr":":1067","server_ip":"192.168.1.1","subnet_mask":"255.255.255.0","pool_start":"192.168.1.100","pool_end":"192.168.1.200","router":"192.168.1.1","dns_servers":"1.1.1.1","lease_seconds":3600}`)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"listen_addr":":1067"`) {
		t.Fatalf("update settings: status=%d body=%s", updated.Code, updated.Body.String())
	}
	created := call(t, handler, http.MethodPost, "/static-leases", `{"mac":"02:00:00:00:00:01","ip":"192.168.1.20","hostname":"printer"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create lease: status=%d body=%s", created.Code, created.Body.String())
	}
	items := call(t, handler, http.MethodGet, "/static-leases", "")
	if items.Code != http.StatusOK || !strings.Contains(items.Body.String(), `"hostname":"printer"`) {
		t.Fatalf("list static leases: status=%d body=%s", items.Code, items.Body.String())
	}
	updatedLease := call(t, handler, http.MethodPut, "/static-leases/1", `{"mac":"02:00:00:00:00:01","ip":"192.168.1.21","hostname":"printer-2"}`)
	if updatedLease.Code != http.StatusNoContent {
		t.Fatalf("update lease: status=%d body=%s", updatedLease.Code, updatedLease.Body.String())
	}
	deleted := call(t, handler, http.MethodDelete, "/static-leases/1", "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete lease: status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestDHCPActionAndServiceError(t *testing.T) {
	handler, service := newTestHandler(t)
	response := call(t, handler, http.MethodPost, "/action", `{"action":"reload"}`)
	if response.Code != http.StatusOK || service.lastAction != "reload" || !strings.Contains(response.Body.String(), `"state":"reload"`) {
		t.Fatalf("action: status=%d service=%+v body=%s", response.Code, service, response.Body.String())
	}
	service.fail = true
	failed := call(t, handler, http.MethodPut, "/settings", `{}`)
	if failed.Code != http.StatusBadRequest || !strings.Contains(failed.Body.String(), `"message":"configuration rejected"`) {
		t.Fatalf("service failure: status=%d body=%s", failed.Code, failed.Body.String())
	}
}

func TestDHCPConfigurationAPIPropagatesAndHonorsContext(t *testing.T) {
	type contextKey struct{}
	handler, service := newTestHandler(t)
	body := `{"enabled":false,"listen_addr":":1067","server_ip":"192.168.1.1","subnet_mask":"255.255.255.0","pool_start":"192.168.1.100","pool_end":"192.168.1.200","router":"192.168.1.1","dns_servers":"1.1.1.1","lease_seconds":3600}`
	request := httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), contextKey{}, "request-trace"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.ctx == nil || service.ctx.Value(contextKey{}) != "request-trace" {
		t.Fatalf("status=%d context=%v body=%s", response.Code, service.ctx, response.Body.String())
	}

	otherHandler, otherService := newTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader(body)).WithContext(ctx)
	cancelledResponse := httptest.NewRecorder()
	otherHandler.ServeHTTP(cancelledResponse, cancelled)
	if otherService.ctx == nil || !errors.Is(otherService.ctx.Err(), context.Canceled) {
		t.Fatalf("cancelled context was not propagated: %v", otherService.ctx)
	}
	stored, err := otherService.repo.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stored.ListenAddr != ":67" {
		t.Fatalf("cancelled settings update was persisted: %+v", stored)
	}
}

func TestDHCPStatusAPIPropagatesCancelledContext(t *testing.T) {
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

func TestStandaloneServesPageManifestAssetsAndSettingsAPI(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "standalone.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := NewLocalService(context.Background(), repo)
	defer service.Close()
	handler := New(repo, service).StandaloneHandler()
	for _, test := range []struct{ path, contains string }{
		{"/", `name="api-base"`},
		{"/v1/dhcp/manifest.json", `assets/module.js`},
		{"/v1/dhcp/assets/module.js", `createAPI`},
		{"/v1/dhcp/assets/index.js", `runtime.createAPI`},
		{"/v1/dhcp/settings", `"listen_addr"`},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.contains) {
			t.Fatalf("GET %s status=%d body=%q", test.path, response.Code, response.Body.String())
		}
	}
}

func TestStandaloneServiceOwnsDHCPRuntimeLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "runtime.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	settings, err := repo.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ListenAddr = "127.0.0.1:0"
	settings.Enabled = true
	if err := repo.UpdateSettings(context.Background(), *settings); err != nil {
		t.Fatal(err)
	}
	service := NewLocalService(context.Background(), repo)
	defer service.Close()
	if err := service.DHCPAction(context.Background(), "start"); err != nil {
		t.Fatal(err)
	}
	status, err := service.DHCPRuntimeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := status.(map[string]any)
	runtimeStatus := state["runtime"].(RuntimeStatus)
	if !runtimeStatus.Running || runtimeStatus.ListenAddr == "" || state["enabled"] != true {
		t.Fatalf("runtime did not start: %#v", status)
	}
	if err := service.DHCPAction(context.Background(), "stop"); err != nil {
		t.Fatal(err)
	}
	status, err = service.DHCPRuntimeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state = status.(map[string]any)
	if state["enabled"] != false || state["runtime"].(RuntimeStatus).Running {
		t.Fatalf("runtime did not stop: %#v", status)
	}
}

func TestStandaloneEntryReusesModulePages(t *testing.T) {
	index, err := moduleWeb.ReadFile("assets/index.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(index)
	if strings.Contains(source, "getSettings(") || strings.Contains(source, "<form") || strings.Contains(source, "staticLeases.map") {
		t.Fatalf("standalone entry must not reimplement DHCP business pages")
	}
	for _, want := range []string{"runtime.createAPI", "matched.route.load()", "manifest.navigation"} {
		if !strings.Contains(source, want) {
			t.Fatalf("standalone entry missing shared runtime behavior %q", want)
		}
	}
}
