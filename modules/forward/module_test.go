package forward_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	forward "github.com/lsongdev/ohmyserver/modules/forward"
	forwardweb "github.com/lsongdev/ohmyserver/modules/forward/web"
	_ "modernc.org/sqlite"
)

type apiService struct{ repo *forwardweb.Repository }

func (s apiService) CreateForwardListener(ctx context.Context, item *forward.Listener) error {
	return s.repo.Create(ctx, item)
}
func (s apiService) UpdateForwardListener(ctx context.Context, item forward.Listener) error {
	return s.repo.Update(ctx, item)
}
func (s apiService) DeleteForwardListener(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func (s apiService) ForwardListenerAction(ctx context.Context, id int64, action string) error {
	switch action {
	case "activate":
		return s.repo.SetStatus(ctx, id, "active")
	case "deactivate":
		return s.repo.SetStatus(ctx, id, "inactive")
	case "restart":
		return s.repo.SetStatus(ctx, id, "active")
	default:
		return http.ErrNotSupported
	}
}

func newTestModule(t *testing.T) http.Handler {
	t.Helper()
	db, err := sql.Open("sqlite", "file:forward-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := forwardweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return forwardweb.New(repo, apiService{repo: repo}).Handler()
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestForwardListenerAPICRUDAndLifecycle(t *testing.T) {
	handler := newTestModule(t)
	created := request(t, handler, http.MethodPost, "/listeners", `{"name":"edge","protocol":"tcp","listen_port":18080,"forward_host":"127.0.0.1","forward_port":8080}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"status":"active"`) {
		t.Fatalf("create: status=%d body=%s", created.Code, created.Body.String())
	}
	if !strings.Contains(created.Body.String(), `"id":1`) {
		t.Fatalf("create did not return persisted listener: %s", created.Body.String())
	}

	updated := request(t, handler, http.MethodPut, "/listeners/1", `{"name":"edge-v2","protocol":"udp","listen_port":18081,"forward_host":"localhost","forward_port":8081}`)
	if updated.Code != http.StatusNoContent {
		t.Fatalf("update: status=%d body=%s", updated.Code, updated.Body.String())
	}
	listed := request(t, handler, http.MethodGet, "/listeners", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"name":"edge-v2"`) || !strings.Contains(listed.Body.String(), `"protocol":"udp"`) {
		t.Fatalf("list after update: status=%d body=%s", listed.Code, listed.Body.String())
	}

	deactivated := request(t, handler, http.MethodPost, "/listeners/1/actions/deactivate", "")
	if deactivated.Code != http.StatusNoContent {
		t.Fatalf("deactivate: status=%d body=%s", deactivated.Code, deactivated.Body.String())
	}
	got := request(t, handler, http.MethodGet, "/listeners/1", "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"status":"inactive"`) {
		t.Fatalf("get after deactivate: status=%d body=%s", got.Code, got.Body.String())
	}

	deleted := request(t, handler, http.MethodDelete, "/listeners/1", "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	missing := request(t, handler, http.MethodGet, "/listeners/1", "")
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"error"`) {
		t.Fatalf("missing listener: status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestForwardListenerAPIValidatesInput(t *testing.T) {
	response := request(t, newTestModule(t), http.MethodPost, "/listeners", `{"name":"invalid","protocol":"icmp","listen_port":99999}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"invalid_listener"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStandaloneAssets(t *testing.T) {
	db, err := sql.Open("sqlite", "file:forward-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := forwardweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := forwardweb.NewLocalService(repo)
	defer service.Close()
	handler := forwardweb.New(repo, service).StandaloneHandler()
	for _, path := range []string{"/", "/v1/forward/manifest.json", "/v1/forward/assets/module.js", "/v1/forward/assets/index.js", "/v1/forward/listeners"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}
