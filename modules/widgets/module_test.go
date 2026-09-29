package widgets_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	widgetsweb "github.com/lsongdev/ohmyserver/modules/widgets/web"

	_ "modernc.org/sqlite"
)

func TestWidgetsAPIValidationAndCRUD(t *testing.T) {
	db, err := sql.Open("sqlite", "file:widgets-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := widgetsweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := widgetsweb.New(repo).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if response := call(http.MethodPost, "/widgets", `{"name":" ","type":"clock"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid widget: status=%d body=%s", response.Code, response.Body.String())
	}
	created := call(http.MethodPost, "/widgets", `{"name":"Clock","type":"clock","config":"{}","sort_order":2,"enabled":true}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"id":1`) {
		t.Fatalf("create: status=%d body=%s", created.Code, created.Body.String())
	}
	if response := call(http.MethodPut, "/widgets/nope", `{"name":"Clock","type":"clock"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid id: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPut, "/widgets/1", `{"name":"Updated","type":"clock","enabled":true}`); response.Code != http.StatusNoContent {
		t.Fatalf("update: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/widgets", ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"Updated"`) {
		t.Fatalf("list updated: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodDelete, "/widgets/1", ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStandaloneAssetsAndAPI(t *testing.T) {
	db, err := sql.Open("sqlite", "file:widgets-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := widgetsweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := widgetsweb.New(repo).Handler()
	for _, path := range []string{"/", "/manifest.json", "/assets/module.js", "/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/widgets", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET widgets API: status=%d body=%s", response.Code, response.Body.String())
	}
}
