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

func TestHandlerServesPageModuleAssetsAndProjectAPI(t *testing.T) {
	db, err := sql.Open("sqlite", "file:projects-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := New(repo, nil).Handler()
	for path, contains := range map[string]string{
		"/":                    "api-base",
		"/manifest.json":       "assets/module.js",
		"/assets/index.js":     "lsong.org/scripts/react/index.js",
		"/assets/module.js":    "createAPI",
		"/assets/projects.css": ".projects-page",
		"/projects":            `"data":[]`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), contains) {
			t.Errorf("GET %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
