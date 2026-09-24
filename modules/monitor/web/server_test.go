package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
	_ "modernc.org/sqlite"
)

type testCollector struct{ err error }

func (c testCollector) CollectMonitorStats(context.Context) (*monitor.Stats, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &monitor.Stats{Hostname: "local", CPUUsage: 12.5}, nil
}

func TestMonitorAPIHistoryAndCurrent(t *testing.T) {
	db, err := sql.Open("sqlite", "file:monitor-module-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	ctx := context.Background()
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	for i, host := range []string{"nas", "router"} {
		if err := repo.Insert(ctx, &monitor.Stats{Hostname: host, Timestamp: time.Now().Add(time.Duration(i) * time.Second), CPUUsage: float64(10 + i)}); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(repo, testCollector{}).Handler()
	call := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	if response := call("/hosts"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "router") || !strings.Contains(response.Body.String(), "nas") {
		t.Fatalf("hosts: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call("/hosts/nas/history?duration=1h"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"sample_interval_seconds"`) || !strings.Contains(response.Body.String(), `"hostname":"nas"`) {
		t.Fatalf("history: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call("/hosts/nas/history?duration=60d"); response.Code != http.StatusBadRequest {
		t.Fatalf("too-long history: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call("/current"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cpu_usage":12.5`) {
		t.Fatalf("current: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMonitorCurrentCollectorFailure(t *testing.T) {
	db, err := sql.Open("sqlite", "file:monitor-module-error-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	New(repo, testCollector{err: errors.New("collector down")}).Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/current", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_unavailable") {
		t.Fatalf("collector failure: status=%d body=%s", w.Code, w.Body.String())
	}
}
