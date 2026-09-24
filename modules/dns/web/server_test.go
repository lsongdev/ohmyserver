package web

import (
	"context"
	"database/sql"
	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

type apiService struct {
	Service
	repo       *Repository
	lastAction string
	ctx        context.Context
}

func (s *apiService) UpdateDNSConfiguration(ctx context.Context, item dnsmodule.Settings) error {
	s.ctx = ctx
	return s.repo.UpdateSettings(ctx, item)
}
func (s *apiService) DNSRuntimeStatus(ctx context.Context) (any, error) {
	s.ctx = ctx
	return map[string]string{"state": "running"}, ctx.Err()
}
func (s *apiService) ReloadDNS(ctx context.Context) error { s.ctx = ctx; return ctx.Err() }
func (s *apiService) DNSAction(_ context.Context, action string) error {
	s.lastAction = action
	return nil
}
func newTestHandler(t *testing.T) (http.Handler, *apiService) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:dns-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	ctx := context.Background()
	if err := repo.InitializeModule(ctx); err != nil {
		t.Fatal(err)
	}
	service := &apiService{repo: repo}
	return New(repo, service, t.TempDir()).Handler(), service
}

func call(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestDNSProviderAPIDoesNotExposeCredentials(t *testing.T) {
	handler, _ := newTestHandler(t)
	created := call(t, handler, http.MethodPost, "/providers", `{"name":"cloud","type":"cloudflare","config":{"token":"secret-token"}}`)
	if created.Code != http.StatusCreated || strings.Contains(created.Body.String(), "secret-token") || strings.Contains(created.Body.String(), `"config"`) {
		t.Fatalf("create provider leaked credentials or failed: status=%d body=%s", created.Code, created.Body.String())
	}
	listed := call(t, handler, http.MethodGet, "/providers", "")
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "secret-token") || strings.Contains(listed.Body.String(), `"config"`) {
		t.Fatalf("list provider leaked credentials or failed: status=%d body=%s", listed.Code, listed.Body.String())
	}
	invalid := call(t, handler, http.MethodPost, "/providers", `{"name":"missing-config","type":"cloudflare"}`)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"code":"invalid_provider"`) {
		t.Fatalf("invalid provider: status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestProviderClientValidatesContextAndPersistedProviderType(t *testing.T) {
	handler, service := newTestHandler(t)
	_ = handler
	if _, err := NewProviderClient(context.Background(), service.repo, 999); err == nil {
		t.Fatal("expected missing provider error")
	}
	if _, err := service.repo.Create(context.Background(), "unsupported", "unknown-vendor", `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProviderClient(context.Background(), service.repo, 1); err == nil || !strings.Contains(err.Error(), "unsupported DNS provider") {
		t.Fatalf("unexpected unsupported provider error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewProviderClient(ctx, service.repo, 1); err != context.Canceled {
		t.Fatalf("got error %v, want context.Canceled", err)
	}
}

func TestDNSZoneRecordAndServiceActionAPI(t *testing.T) {
	handler, _ := newTestHandler(t)
	zone := call(t, handler, http.MethodPost, "/zones", `{"name":"example.test","description":"test zone"}`)
	if zone.Code != http.StatusCreated || !strings.Contains(zone.Body.String(), `"enabled":true`) {
		t.Fatalf("create zone: status=%d body=%s", zone.Code, zone.Body.String())
	}
	createdRecord := call(t, handler, http.MethodPost, "/zones/1/records", `{"name":"www","type":"A","ttl":300,"value":"192.0.2.10","enabled":true}`)
	if createdRecord.Code != http.StatusCreated || !strings.Contains(createdRecord.Body.String(), `"zone_id":1`) {
		t.Fatalf("create record: status=%d body=%s", createdRecord.Code, createdRecord.Body.String())
	}
	listed := call(t, handler, http.MethodGet, "/zones/1/records", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"value":"192.0.2.10"`) {
		t.Fatalf("list records: status=%d body=%s", listed.Code, listed.Body.String())
	}
	badID := call(t, handler, http.MethodGet, "/zones/invalid/records", "")
	if badID.Code != http.StatusBadRequest || !strings.Contains(badID.Body.String(), `"code":"invalid_id"`) {
		t.Fatalf("invalid path id: status=%d body=%s", badID.Code, badID.Body.String())
	}
	status := call(t, handler, http.MethodGet, "/status", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"state":"running"`) {
		t.Fatalf("status: status=%d body=%s", status.Code, status.Body.String())
	}
	acted := call(t, handler, http.MethodPost, "/action", `{"action":"reload"}`)
	if acted.Code != http.StatusOK || !strings.Contains(acted.Body.String(), `"state":"running"`) {
		t.Fatalf("action: status=%d body=%s", acted.Code, acted.Body.String())
	}
}

func TestDNSWriteAPIPropagatesRequestContext(t *testing.T) {
	type contextKey struct{}
	handler, service := newTestHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/zones", strings.NewReader(`{"name":"context.test"}`))
	request = request.WithContext(context.WithValue(request.Context(), contextKey{}, "request-trace"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || service.ctx == nil || service.ctx.Value(contextKey{}) != "request-trace" {
		t.Fatalf("status=%d context=%v body=%s", response.Code, service.ctx, response.Body.String())
	}
}

func TestDNSWriteAPIHonorsCancelledRequest(t *testing.T) {
	handler, service := newTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodPost, "/zones", strings.NewReader(`{"name":"cancelled.test"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code == http.StatusCreated {
		t.Fatalf("status=%d service context=%v body=%s", response.Code, service.ctx, response.Body.String())
	}
	zones, err := service.repo.Zones(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 0 {
		t.Fatalf("cancelled request created zones: %#v", zones)
	}
}

func TestDNSStatusAPIPropagatesCancelledContext(t *testing.T) {
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
