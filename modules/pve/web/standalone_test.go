package web

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type emptyService struct{ Service }

func (emptyService) ListPVE(context.Context) ([]DTO, error) { return []DTO{}, nil }
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, e := sql.Open("sqlite", "file:pve-standalone-test?mode=memory&cache=shared")
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
func TestStandaloneServesPageManifestModuleAndAPI(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	if e := repo.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	h := New(NewLocalService(repo)).StandaloneHandler()
	for _, tc := range []struct{ path, contains string }{{"/", "/v1/pve/assets/index.js"}, {"/v1/pve/manifest.json", "assets/module.js"}, {"/v1/pve/assets/module.js", "defineModule"}, {"/v1/pve/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/v1/pve/servers", `"data":[]`}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}
