package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

type apiRuntime struct {
	section, action, service string
	reads, actions, tests    int
}

func (r *apiRuntime) ReadOpenWrt(_ context.Context, _ int64, section string) (any, error) {
	r.reads++
	r.section = section
	return map[string]string{"section": section}, nil
}
func (r *apiRuntime) TestOpenWrt(context.Context, int64) error { r.tests++; return nil }
func (r *apiRuntime) ActOpenWrt(_ context.Context, _ int64, action, service string) error {
	r.actions++
	r.action, r.service = action, service
	return nil
}

func TestOpenWrtModuleOwnsCRUDAndProtectsCredentials(t *testing.T) {
	db, err := sql.Open("sqlite", "file:openwrt-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	runtime := &apiRuntime{}
	handler := New(repo, runtime).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	created := call(http.MethodPost, "/routers", `{"name":"lab","host":" router.local ","username":"root","password":"secret","protocol":"https"}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"port":443`) || strings.Contains(created.Body.String(), "secret") {
		t.Fatalf("create and redact: status=%d body=%s", created.Code, created.Body.String())
	}
	if response := call(http.MethodPut, "/routers/1", `{"name":"lab","host":"router.local","username":"root","protocol":"https","port":443}`); response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("update: status=%d body=%s", response.Code, response.Body.String())
	}
	stored, err := repo.Get(context.Background(), 1)
	if err != nil || stored.Password != "secret" {
		t.Fatalf("omitted password should remain unchanged: router=%+v err=%v", stored, err)
	}
	if response := call(http.MethodGet, "/routers/1/firewall", ""); response.Code != http.StatusOK || runtime.section != "firewall" {
		t.Fatalf("firewall read: status=%d body=%s section=%q", response.Code, response.Body.String(), runtime.section)
	}
	if response := call(http.MethodPost, "/routers/1/actions", `{"action":"factory_reset"}`); response.Code != http.StatusUnprocessableEntity || runtime.actions != 0 {
		t.Fatalf("invalid action: status=%d body=%s calls=%d", response.Code, response.Body.String(), runtime.actions)
	}
	if response := call(http.MethodPost, "/routers/1/actions", `{"action":"reboot"}`); response.Code != http.StatusOK || runtime.actions != 1 || runtime.action != "reboot" {
		t.Fatalf("reboot action: status=%d body=%s action=%q", response.Code, response.Body.String(), runtime.action)
	}
}

func TestHandlerServesUIManifestAssetsAndAPI(t *testing.T) {
	db, err := sql.Open("sqlite", "file:openwrt-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := New(repo, &apiRuntime{}).Handler()
	for path, contains := range map[string]string{
		"/":                              "api-base",
		"/manifest.json":      "assets/module.js",
		"/assets/index.js":    "lsong.org/scripts/react/index.js",
		"/assets/openwrt.css": ".openwrt-standalone-header",
		"/routers":            `"data":[]`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), contains) {
			t.Errorf("GET %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
