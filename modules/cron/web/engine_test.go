package web

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestEngineRunsAndRecordsJobsThenClosesWorker(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "cron.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	started := make(chan struct{})
	engine := NewEngine(context.Background(), NewRepository(db), func(ctx context.Context, _ Job) (string, error) {
		close(started)
		<-ctx.Done()
		return "partial output", ctx.Err()
	})
	if err := engine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	job, err := engine.CreateCronJobAPI(context.Background(), Input{
		Name: "manual", Type: "once", Schedule: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Command: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.RunCronJobNow(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	items, err := engine.CronJobExecutions(context.Background(), job.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != "failed" || items[0].Output != "partial output" || items[0].Error != context.Canceled.Error() {
		t.Fatalf("unexpected execution record: %+v", items)
	}
}
