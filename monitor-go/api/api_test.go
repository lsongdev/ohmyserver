package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/monitor-go/api"
	"github.com/lsongdev/ohmyserver/monitor-go/collector"
	"github.com/lsongdev/ohmyserver/monitor-go/metrics"
	"github.com/lsongdev/ohmyserver/monitor-go/store"
)

// fake offers hosts "fake:a" and "fake:b". Each reading adds 10 busy
// units out of 40 and 1000 received bytes; "fake:b" fails while broken.
type fake struct {
	mu     sync.Mutex
	n      map[string]int
	broken bool
	hosts  []collector.Host
}

func (*fake) Kind() string { return "fake" }

func (f *fake) Hosts(context.Context) ([]collector.Host, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hosts, nil
}

func (f *fake) Read(_ context.Context, host string) (metrics.Reading, metrics.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if host == "fake:b" && f.broken {
		return metrics.Reading{}, metrics.Info{}, errors.New("connection refused")
	}
	f.n[host]++
	n := uint64(f.n[host])
	return metrics.Reading{
		Time: time.Now(), CPUBusy: float64(10 * n), CPUTotal: float64(40 * n), MemTotal: 1000, MemUsed: 250,
		NetRecv: 1000 * n, Disks: []metrics.Disk{{Mount: "/", Device: "/dev/sda1", Total: 100, Used: 30}},
	}, metrics.Info{Hostname: host}, nil
}

func newService(t *testing.T, f *fake) (*api.Service, http.Handler) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	svc := api.NewService(st, api.Options{Collectors: []collector.Collector{f}, NoLocal: true, Interval: 50 * time.Millisecond})
	svc.Start()
	t.Cleanup(svc.Close)
	return svc, svc.Handler()
}

func get(t *testing.T, h http.Handler, method, path string, want int, out any) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	if w.Code != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, path, w.Code, want, w.Body)
	}
	if out != nil {
		var env struct{ Data json.RawMessage }
		json.Unmarshal(w.Body.Bytes(), &env)
		if err := json.Unmarshal(env.Data, out); err != nil {
			t.Fatal(err)
		}
	}
}

// waitFor polls the host list until ok.
func waitFor(t *testing.T, h http.Handler, ok func([]api.HostView) bool) []api.HostView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var hosts []api.HostView
		get(t, h, "GET", "/hosts?spark=1", 200, &hosts)
		if ok(hosts) {
			return hosts
		}
		if time.Now().After(deadline) {
			t.Fatalf("hosts %+v", hosts)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSampling(t *testing.T) {
	f := &fake{n: map[string]int{}, broken: true, hosts: []collector.Host{{ID: "fake:a", Name: "A", Kind: "fake"}, {ID: "fake:b", Name: "B", Kind: "fake"}}}
	_, h := newService(t, f)
	hosts := waitFor(t, h, func(hs []api.HostView) bool {
		return len(hs) == 2 && hs[0].Status == api.StatusOnline && len(hs[0].Spark.CPU) > 0 && hs[1].Status == api.StatusOffline
	})
	a := hosts[0]
	if a.Latest.CPU != 25 || a.Latest.NetRecv <= 0 || a.Latest.DiskUsed != 30 || a.Info.Hostname != "fake:a" || len(a.Disks) != 1 || a.Spark.Mem[0] != 25 {
		t.Fatalf("a = %+v latest %+v", a, a.Latest)
	}
	if hosts[1].Error != "connection refused" {
		t.Fatalf("b = %+v", hosts[1])
	}
	f.mu.Lock()
	f.broken = false
	f.mu.Unlock()
	waitFor(t, h, func(hs []api.HostView) bool { return hs[1].Status == api.StatusOnline })

	var series struct {
		Step   int64            `json:"step"`
		Points []metrics.Sample `json:"points"`
	}
	get(t, h, "GET", "/hosts/fake:a/series?range=1h", 200, &series)
	if len(series.Points) == 0 || series.Step != 12 || series.Points[0].CPU != 25 {
		t.Fatalf("series %+v", series)
	}
	get(t, h, "GET", "/hosts/fake:a/series?range=2w", 400, nil)
	get(t, h, "GET", "/hosts/nope/series?range=1h", 404, nil)

	var sum api.Summary
	get(t, h, "GET", "/summary", 200, &sum)
	if sum.Hosts != 2 || sum.Online != 2 {
		t.Fatalf("summary %+v", sum)
	}

	// Disabling stops reading; history stays.
	var v api.HostView
	get(t, h, "POST", "/hosts/fake:a/disable", 200, &v)
	if v.Status != api.StatusDisabled {
		t.Fatalf("disabled %+v", v)
	}
	f.mu.Lock()
	before := f.n["fake:a"]
	f.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	f.mu.Lock()
	after := f.n["fake:a"]
	f.mu.Unlock()
	if after != before {
		t.Fatalf("disabled host read %d more times", after-before)
	}
	get(t, h, "DELETE", "/hosts/fake:a", 409, nil) // still offered: disable instead
	get(t, h, "POST", "/hosts/fake:a/enable", 200, nil)

	// A host its collector stops offering keeps its history until deleted.
	f.mu.Lock()
	f.hosts = f.hosts[:1]
	f.mu.Unlock()
	waitFor(t, h, func(hs []api.HostView) bool { return len(hs) == 2 && hs[1].Status == api.StatusGone })
	get(t, h, "DELETE", "/hosts/fake:b", 204, nil)
	waitFor(t, h, func(hs []api.HostView) bool { return len(hs) == 1 })
}

func TestLocalCollector(t *testing.T) {
	db, _ := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db")+"?_pragma=busy_timeout(5000)")
	defer db.Close()
	st, _ := store.New(context.Background(), db)
	svc := api.NewService(st, api.Options{Interval: 300 * time.Millisecond})
	svc.Start()
	defer svc.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		sum, err := svc.Summary(context.Background())
		if err == nil && sum.Local != nil && sum.Local.Status == api.StatusOnline {
			if sum.Local.Latest.MemTotal == 0 || len(sum.Local.Disks) == 0 {
				t.Fatalf("local %+v", sum.Local)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("summary %+v %v", sum, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
