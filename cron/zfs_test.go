package cron

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cronapi "github.com/lsongdev/ohmyserver/modules/cron/api"
	"github.com/lsongdev/ohmyserver/modules/cron/runner"
	"github.com/lsongdev/ohmyserver/modules/cron/store"
	cronweb "github.com/lsongdev/ohmyserver/modules/cron/web"
	zfsapi "github.com/lsongdev/ohmyserver/modules/zfsmgr/api"
	"github.com/lsongdev/ohmyserver/modules/zfsmgr/system"
)

// zfsFake is one dataset with a daily policy and no snapshots.
type zfsFake struct{ fail string }

func (f *zfsFake) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	switch {
	case strings.HasPrefix(line, "zfs list -Hp -t filesystem,volume"):
		return []byte("data\tfilesystem\t1\t1\t1\t0\t/data\tyes\tlz4\t0\n"), nil
	case strings.HasPrefix(line, "zfs get -Hp"):
		return []byte("data\thourly=0,daily=7,weekly=0,monthly=0\tlocal\n"), nil
	case strings.HasPrefix(line, "zfs snapshot") && f.fail != "":
		return nil, &system.ExitError{Command: name, Code: 1, Output: f.fail}
	}
	return nil, nil
}

func (f *zfsFake) Lines(context.Context, func(string) bool, string, ...string) error { return nil }

func zfsSetup(t *testing.T, f *zfsFake) (*ZFSJob, *cronapi.Service) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ohmyserver.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	yes := true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	job := &ZFSJob{}
	service := zfsapi.New(zfsapi.Options{Runner: f, Installed: &yes, Logger: logger, Trigger: job})
	scheduler, err := cronweb.New(ctx, db, cronweb.Options{Runners: []runner.Runner{ZFSRunner{Service: service}}, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { scheduler.Close() })
	job.Cron = scheduler.Service
	return job, scheduler.Service
}

func finished(t *testing.T, c *cronapi.Service, id int64) store.Run {
	t.Helper()
	for range 200 {
		runs, err := c.Runs(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) > 0 && runs[0].Status != store.StatusRunning {
			full, err := c.Run(context.Background(), runs[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			return full
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the run did not finish")
	return store.Run{}
}

func TestZFSJob(t *testing.T) {
	ctx := context.Background()
	f := &zfsFake{}
	job, c := zfsSetup(t, f)

	if status, err := job.Status(ctx); err != nil || status.JobID != 0 {
		t.Fatalf("before Ensure: %+v %v", status, err)
	}
	if err := job.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if err := job.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, _ := c.Jobs(ctx)
	if len(jobs) != 1 || jobs[0].Schedule != "0 * * * *" || jobs[0].NextRun.IsZero() {
		t.Fatalf("jobs: %+v", jobs)
	}
	id := jobs[0].ID

	// A run goes through the module and is recorded with what it did.
	if err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	run := finished(t, c, id)
	if run.Status != store.StatusSucceeded || !strings.Contains(run.Output, "Took data@auto-daily-") {
		t.Fatalf("%+v", run)
	}
	status, _ := job.Status(ctx)
	if status.JobID != id || !status.Enabled || status.RunStatus != store.StatusSucceeded || status.NextRun == nil {
		t.Fatalf("%+v", status)
	}

	// What failed fails the job, and the output says why.
	f.fail = "out of space"
	if _, err := c.RunNow(ctx, id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	run = finished(t, c, id)
	if run.Status != store.StatusFailed || !strings.Contains(run.Output, "out of space") {
		t.Fatalf("%+v", run)
	}
	f.fail = ""

	// A job turned off stays off: saving a schedule does not run it, and
	// Ensure at the next start does not turn it back on.
	if _, err := c.SetEnabled(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	before, _ := c.Runs(ctx, id)
	if err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := job.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := c.Runs(ctx, id)
	view, _ := c.Job(ctx, id)
	if len(after) != len(before) || view.Enabled {
		t.Fatalf("runs %d → %d, enabled %v", len(before), len(after), view.Enabled)
	}

	// A deleted job comes back.
	if err := c.DeleteJob(ctx, id); err != nil {
		t.Fatal(err)
	}
	if status, _ := job.Status(ctx); status.JobID != 0 {
		t.Fatalf("%+v", status)
	}
	if err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if status, _ := job.Status(ctx); status.JobID == 0 {
		t.Fatal("the job did not come back")
	}
}

func TestZFSTargetIsInternal(t *testing.T) {
	targets, err := ZFSRunner{}.Targets(context.Background())
	if err != nil || len(targets) != 1 || !targets[0].Internal {
		t.Fatalf("%+v %v", targets, err)
	}
}
