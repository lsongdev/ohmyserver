package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
)

func TestStandaloneServesPageManifestModuleAndAPI(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := New(repo, testCollector{}).Handler()
	for _, tc := range []struct{ path, contains string }{{"/", "assets/index.js"}, {"/manifest.json", "assets/module.js"}, {"/assets/module.js", "createAPI"}, {"/assets/index.js", "lsong.org/scripts/react/index.js"}, {"/hosts", `"data":[]`}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("GET %s status=%d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}
func TestTelegrafIngestionStoresModuleStats(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := `{"metrics":[{"name":"cpu","tags":{"host":"agent"},"fields":{"usage_idle":80}},{"name":"mem","tags":{"host":"agent"},"fields":{"total":1000,"used":400,"free":600,"used_percent":40}}]}`
	w := httptest.NewRecorder()
	New(repo, testCollector{}).Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/telegraf", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	got, err := repo.LatestForHost(context.Background(), "agent")
	if err != nil || got.CPUUsage != 20 || got.MemoryUsage != 40 {
		t.Fatalf("stats=%+v err=%v", got, err)
	}
}
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:monitor-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

var _ monitor.Collector = testCollector{}
