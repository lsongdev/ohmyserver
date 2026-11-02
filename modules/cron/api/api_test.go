package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/cron/api"
	"github.com/lsongdev/ohmyserver/modules/cron/runner"
	"github.com/lsongdev/ohmyserver/modules/cron/store"
)

// fakeRunner offers targets "fake:a" and "fake:b". A command "block" runs
// until cancelled; anything else is echoed with the target's name, and
// "fail" exits 2.
type fakeRunner struct {
	mu   sync.Mutex
	runs []string
}

func (*fakeRunner) Kind() string { return "fake" }

func (*fakeRunner) Targets(context.Context) ([]runner.Target, error) {
	return []runner.Target{{ID: "fake:a", Name: "A", Kind: "fake"}, {ID: "fake:b", Name: "B", Kind: "fake"}}, nil
}

func (f *fakeRunner) Run(ctx context.Context, target string, cmd runner.Command, out io.Writer) (int, error) {
	f.mu.Lock()
	f.runs = append(f.runs, target+" "+cmd.Script)
	f.mu.Unlock()
	fmt.Fprintf(out, "%s on %s\n", cmd.Script, target)
	switch cmd.Script {
	case "block":
		<-ctx.Done()
		return -1, ctx.Err()
	case "fail":
		return 2, nil
	}
	return 0, nil
}

type env struct {
	t    *testing.T
	db   *sql.DB
	svc  *api.Service
	fake *fakeRunner
	h    http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{t: t, db: db, fake: &fakeRunner{}}
	e.start()
	return e
}

// start builds the service on the env's database, as after a restart.
func (e *env) start() {
	st, err := store.New(context.Background(), e.db)
	if err != nil {
		e.t.Fatal(err)
	}
	e.svc = api.NewService(st, api.Options{Runners: []runner.Runner{e.fake}})
	if err := e.svc.Start(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(e.svc.Close)
	e.h = e.svc.Handler()
}

func (e *env) call(method, path string, body any, want int, out any) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, httptest.NewRequest(method, path, r))
	if w.Code != want {
		e.t.Fatalf("%s %s: %d, want %d: %s", method, path, w.Code, want, w.Body)
	}
	if out != nil {
		var envelope struct{ Data json.RawMessage }
		json.Unmarshal(w.Body.Bytes(), &envelope)
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			e.t.Fatalf("%s %s: %v: %s", method, path, err, w.Body)
		}
	}
}

// wait polls run id until it has finished.
func (e *env) wait(id int64) store.Run {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var r store.Run
		e.call("GET", fmt.Sprintf("/runs/%d", id), nil, 200, &r)
		if r.Status != store.StatusRunning {
			return r
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("run %d still running: %+v", id, r)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func job(name, target, command string) map[string]any {
	return map[string]any{"name": name, "schedule": "0 3 * * *", "target": target, "command": command, "enabled": true}
}

func TestJobsAndRuns(t *testing.T) {
	e := newEnv(t)
	var j api.JobView
	e.call("POST", "/jobs", job("hello", "fake:a", "hello"), 201, &j)
	if j.NextRun.IsZero() || j.NextRun.Hour() != 3 {
		t.Fatalf("created %+v", j)
	}
	e.call("POST", "/jobs", job("hello", "fake:a", "x"), 409, nil)
	for _, bad := range []map[string]any{
		job("", "fake:a", "x"), job("x", "fake:a", ""), job("x", "nowhere", "x"), job("x", "fake:zzz", "x"),
		{"name": "x", "schedule": "61 * * * *", "target": "fake:a", "command": "x"},
		{"name": "x", "schedule": "@daily", "timezone": "Mars/Base", "target": "fake:a", "command": "x"},
	} {
		e.call("POST", "/jobs", bad, 422, nil)
	}

	var run store.Run
	e.call("POST", "/jobs/1/run", nil, 202, &run)
	run = e.wait(run.ID)
	if run.Status != store.StatusSucceeded || run.Trigger != "manual" || run.Output != "hello on fake:a\n" {
		t.Fatalf("run %+v", run)
	}

	e.call("PUT", "/jobs/1", job("hello", "fake:b", "fail"), 200, &j)
	e.call("POST", "/jobs/1/run", nil, 202, &run)
	if run = e.wait(run.ID); run.Status != store.StatusFailed || run.ExitCode != 2 {
		t.Fatalf("failing run %+v", run)
	}
	var runs []store.Run
	e.call("GET", "/jobs/1/runs", nil, 200, &runs)
	if len(runs) != 2 || runs[0].ID != run.ID || runs[0].Output != "" {
		t.Fatalf("runs %+v", runs)
	}
	var sum api.Summary
	e.call("GET", "/summary", nil, 200, &sum)
	if sum.Jobs != 1 || sum.Enabled != 1 || sum.Failing != 1 {
		t.Fatalf("summary %+v", sum)
	}

	j = api.JobView{} // zero fields are omitted from JSON
	e.call("POST", "/jobs/1/disable", nil, 200, &j)
	if j.Enabled || !j.NextRun.IsZero() {
		t.Fatalf("disabled %+v", j)
	}
	e.call("DELETE", "/jobs/1", nil, 204, nil)
	e.call("GET", fmt.Sprintf("/runs/%d", run.ID), nil, 404, nil)
}

func TestCancelTimeoutAndOverlap(t *testing.T) {
	e := newEnv(t)
	e.call("POST", "/jobs", job("block", "fake:a", "block"), 201, nil)
	var run store.Run
	e.call("POST", "/jobs/1/run", nil, 202, &run)
	e.call("POST", "/jobs/1/run", nil, 409, nil) // already running
	// The runner writes its first line in the background.
	var live store.Run
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		e.call("GET", fmt.Sprintf("/runs/%d", run.ID), nil, 200, &live)
		if live.Output != "" || time.Now().After(deadline) {
			break
		}
	}
	if live.Status != store.StatusRunning || live.Output != "block on fake:a\n" {
		t.Fatalf("live %+v", live)
	}
	var j api.JobView
	e.call("GET", "/jobs/1", nil, 200, &j)
	if j.Running == nil || j.Running.ID != run.ID {
		t.Fatalf("job while running %+v", j)
	}
	e.call("POST", fmt.Sprintf("/runs/%d/cancel", run.ID), nil, 202, nil)
	if run = e.wait(run.ID); run.Status != store.StatusCanceled {
		t.Fatalf("canceled %+v", run)
	}
	e.call("POST", fmt.Sprintf("/runs/%d/cancel", run.ID), nil, 422, nil)

	in := job("block", "fake:a", "block")
	in["timeout"] = 1
	e.call("PUT", "/jobs/1", in, 200, nil)
	e.call("POST", "/jobs/1/run", nil, 202, &run)
	if run = e.wait(run.ID); run.Status != store.StatusTimeout || !strings.Contains(run.Error, "1s") {
		t.Fatalf("timeout %+v", run)
	}
}

// Jobs fire on schedule; runs in progress when the program stops are
// recorded as interrupted.
func TestScheduleAndRestart(t *testing.T) {
	e := newEnv(t)
	in := job("tick", "fake:a", "tick")
	in["schedule"] = "@every 1s"
	e.call("POST", "/jobs", in, 201, nil)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var runs []store.Run
		e.call("GET", "/jobs/1/runs", nil, 200, &runs)
		if len(runs) >= 2 && runs[0].Trigger == "schedule" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runs %+v", runs)
		}
		time.Sleep(50 * time.Millisecond)
	}

	e.call("PUT", "/jobs/1", job("tick", "fake:a", "block"), 200, nil)
	var run store.Run
	e.call("POST", "/jobs/1/run", nil, 202, &run)
	e.svc.Close()
	e.start()
	if run = e.wait(run.ID); run.Status != store.StatusInterrupted {
		t.Fatalf("after restart %+v", run)
	}
}

func TestLocalTargetAndPreview(t *testing.T) {
	e := newEnv(t)
	var targets []runner.Target
	e.call("GET", "/targets", nil, 200, &targets)
	if len(targets) != 3 || targets[0].ID != "local" || targets[1].ID != "fake:a" {
		t.Fatalf("targets %+v", targets)
	}
	e.call("POST", "/jobs", job("sh", "local", "echo $((6*7)); exit 4"), 201, nil)
	var run store.Run
	e.call("POST", "/jobs/1/run", nil, 202, &run)
	if run = e.wait(run.ID); run.Status != store.StatusFailed || run.ExitCode != 4 || run.Output != "42\n" {
		t.Fatalf("local run %+v", run)
	}
	var p struct{ Next []time.Time }
	e.call("GET", "/preview?schedule=0+12+*+*+*&timezone=Asia/Tokyo", nil, 200, &p)
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	if len(p.Next) != 5 || p.Next[0].In(tokyo).Hour() != 12 {
		t.Fatalf("preview %+v", p.Next)
	}
	e.call("GET", "/preview?schedule=nope", nil, 422, nil)
}

func TestOutputIsCapped(t *testing.T) {
	db, _ := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db")+"?_pragma=foreign_keys(1)")
	defer db.Close()
	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	svc := api.NewService(st, api.Options{MaxOutput: 100, Keep: 2})
	svc.Start(context.Background())
	defer svc.Close()
	v, err := svc.CreateJob(context.Background(), store.Job{Name: "loud", Schedule: "@daily", Target: "local", Command: "seq 1 1000"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		run, err := svc.RunNow(context.Background(), v.ID)
		if err != nil {
			t.Fatal(err)
		}
		for {
			if run, _ = svc.Run(context.Background(), run.ID); run.Status != store.StatusRunning {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !strings.HasPrefix(run.Output, "…") || !strings.HasSuffix(run.Output, "999\n1000\n") || len(run.Output) > 150 {
			t.Fatalf("output %q", run.Output)
		}
	}
	if runs, _ := svc.Runs(context.Background(), v.ID); len(runs) != 2 {
		t.Fatalf("kept %d runs", len(runs))
	}
}

func TestEnsureJobCreatesOnceAndKeepsChanges(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "cron.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st, err := store.New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	s := api.NewService(st, api.Options{Runners: []runner.Runner{&fakeRunner{}}})
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	job := store.Job{Name: "managed", Schedule: "@daily", Target: "fake:a", Command: "x", Enabled: true}
	first, err := s.EnsureJob(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetEnabled(ctx, first.ID, false); err != nil {
		t.Fatal(err)
	}
	job.Name, job.Schedule = "other", "@hourly"
	second, err := s.EnsureJob(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.Jobs(ctx)
	if len(jobs) != 1 || second.ID != first.ID || second.Enabled || second.Schedule != "@daily" {
		t.Fatalf("%+v", jobs)
	}
}
