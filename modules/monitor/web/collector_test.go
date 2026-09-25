package web

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
	_ "modernc.org/sqlite"
)

type historicalCollectorFunc func(context.Context) ([]monitor.Stats, error)

type signalCollector chan struct{}

func (c signalCollector) CollectMonitorStats(context.Context) (*monitor.Stats, error) {
	c <- struct{}{}
	return &monitor.Stats{Hostname: "local"}, nil
}

func (f historicalCollectorFunc) CollectHistoricalStats(ctx context.Context) ([]monitor.Stats, error) {
	return f(ctx)
}

func TestCollectionWorkerCollectOnceKeepsSuccessfulSources(t *testing.T) {
	db, err := sql.Open("sqlite", "file:monitor-worker-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	ctx := context.Background()
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	worker := NewCollectionWorker(repo, testCollector{}, historicalCollectorFunc(func(context.Context) ([]monitor.Stats, error) {
		return []monitor.Stats{{Hostname: "remote", CPUUsage: 4}}, errors.New("one host failed")
	}), 0)
	if err := worker.CollectOnce(ctx); err == nil {
		t.Fatal("expected partial collection error")
	}
	for _, host := range []string{"local", "remote"} {
		if _, err := repo.LatestForHost(ctx, host); err != nil {
			t.Fatalf("successful source %q was not persisted: %v", host, err)
		}
	}
}

func TestCollectionServiceOwnsStartAndCloseLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite", "file:monitor-collection-service-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	collected := make(signalCollector, 1)
	service := NewCollectionService(context.Background(), repo, collected, nil, time.Hour)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	if err := service.Start(); err == nil {
		t.Fatal("second Start succeeded")
	}
	select {
	case <-collected:
	case <-time.After(2 * time.Second):
		t.Fatal("initial collection did not run")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
	if err := service.Start(); err == nil {
		t.Fatal("Start after Close succeeded")
	}
}
