package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	uptime "github.com/lsongdev/ohmyserver/modules/uptime"
	_ "modernc.org/sqlite"
)

type apiChecker struct{ repo *Repository }

func (c apiChecker) CheckNow(ctx context.Context, id int64) (*uptime.Check, error) {
	check := &uptime.Check{MonitorID: id, Status: "up", ResponseTime: 12, Message: "HTTP 200", CheckedAt: time.Now()}
	return check, c.repo.InsertCheck(ctx, check)
}

func newTestModule(t *testing.T) *Module {
	t.Helper()
	db, err := sql.Open("sqlite", "file:uptime-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return New(repo, apiChecker{repo: repo})
}

func newTestHandler(t *testing.T) http.Handler {
	return newTestModule(t).Handler()
}

func call(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestMonitorAPIAndCheckHistory(t *testing.T) {
	handler := newTestHandler(t)
	created := call(t, handler, http.MethodPost, "/monitors", `{"name":"site","type":"http","target":"https://example.test","enabled":true}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"id":1`) {
		t.Fatalf("create: status=%d body=%s", created.Code, created.Body.String())
	}
	if invalid := call(t, handler, http.MethodPost, "/monitors", `{"name":"","type":"icmp"}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid create: status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	checked := call(t, handler, http.MethodPost, "/monitors/1/actions/check", "")
	if checked.Code != http.StatusOK || !strings.Contains(checked.Body.String(), `"status":"up"`) {
		t.Fatalf("check: status=%d body=%s", checked.Code, checked.Body.String())
	}
	history := call(t, handler, http.MethodGet, "/monitors/1/checks?limit=10", "")
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"response_time":12`) {
		t.Fatalf("history: status=%d body=%s", history.Code, history.Body.String())
	}

	toggled := call(t, handler, http.MethodPost, "/monitors/1/actions/toggle", "")
	if toggled.Code != http.StatusNoContent {
		t.Fatalf("toggle: status=%d body=%s", toggled.Code, toggled.Body.String())
	}
	list := call(t, handler, http.MethodGet, "/monitors", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"enabled":false`) || !strings.Contains(list.Body.String(), `"last_check"`) {
		t.Fatalf("list after check/toggle: status=%d body=%s", list.Code, list.Body.String())
	}
}

func TestHandlerServesModuleManifestAndEntry(t *testing.T) {
	handler := newTestHandler(t)
	for _, path := range []string{"/manifest.json", "/assets/index.js"} {
		response := call(t, handler, http.MethodGet, path, "")
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestStandaloneServesPageAndAPI(t *testing.T) {
	handler := newTestModule(t).Handler()
	for _, path := range []string{"/", "/manifest.json", "/assets/module.js", "/assets/index.js"} {
		response := call(t, handler, http.MethodGet, path, "")
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}
