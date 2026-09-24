package settings_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settingsweb "github.com/lsongdev/ohmyserver/modules/settings/web"
	_ "modernc.org/sqlite"
)

func TestSettingsAPIValidationAndCRUD(t *testing.T) {
	db, err := sql.Open("sqlite", "file:settings-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := settingsweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := settingsweb.New(repo).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	if response := call(http.MethodPost, "/", `{"name":"bad","type":"secret","value":"x"}`); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid type: status=%d body=%s", response.Code, response.Body.String())
	}
	created := call(http.MethodPost, "/", `{"name":"site.title","type":"string","value":"hello","description":"title"}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"name":"site.title"`) {
		t.Fatalf("create: status=%d body=%s", created.Code, created.Body.String())
	}
	if response := call(http.MethodGet, "/", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"value":"hello"`) {
		t.Fatalf("list: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPut, "/1", `{"value":"updated"}`); response.Code != http.StatusOK {
		t.Fatalf("update: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodDelete, "/1", ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":[]`) {
		t.Fatalf("empty list: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerServesModuleManifestAndEntry(t *testing.T) {
	db, err := sql.Open("sqlite", "file:settings-assets-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := settingsweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := settingsweb.New(repo).Handler()
	for _, path := range []string{"/manifest.json", "/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestStandaloneServesPageManifestAndAPI(t *testing.T) {
	db, err := sql.Open("sqlite", "file:settings-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := settingsweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := settingsweb.New(repo).StandaloneHandler()
	for _, path := range []string{"/", "/v1/settings/manifest.json", "/v1/settings/assets/module.js", "/v1/settings/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}
