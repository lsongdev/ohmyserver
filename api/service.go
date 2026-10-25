// Package api schedules jobs, runs them on their targets and keeps their
// history, and serves all of it as a JSON API with relative routes.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/cron-go/runner"
	"github.com/lsongdev/cron-go/schedule"
	"github.com/lsongdev/cron-go/store"
)

var (
	// ErrInvalid marks a job that fails validation.
	ErrInvalid = errors.New("invalid job")
	// ErrRunning reports a job that is already running.
	ErrRunning = errors.New("the job is already running")
)

// Options configures the service; zero values select the defaults.
type Options struct {
	// Runners run commands; runner.Local is added unless NoLocal is set.
	Runners []runner.Runner
	NoLocal bool
	Keep    int // runs kept per job (100)
	// MaxOutput bounds the output kept per run; the end is kept (256 KiB).
	MaxOutput int
	Logger    *slog.Logger
}

// JobView is a job with its schedule and run state.
type JobView struct {
	store.Job
	NextRun time.Time  `json:"next_run,omitzero"`
	Running *store.Run `json:"running,omitempty"`  // the run in progress
	LastRun *store.Run `json:"last_run,omitempty"` // the latest finished or running run
}

// Summary counts jobs for an overview.
type Summary struct {
	Jobs    int `json:"jobs"`
	Enabled int `json:"enabled"`
	Running int `json:"running"`
	Failing int `json:"failing"` // jobs whose latest run did not succeed
}

// Service schedules and runs jobs.
type Service struct {
	store   *store.SQLite
	runners map[string]runner.Runner
	opts    Options
	log     *slog.Logger

	ctx    context.Context // ends at Close
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	timers  map[int64]*time.Timer
	next    map[int64]time.Time
	active  map[int64]*active // by job ID
	started bool
}

// active is a run in progress.
type active struct {
	run    store.Run
	out    *tail
	cancel context.CancelCauseFunc
}

var (
	errCanceled = errors.New("canceled")
	errShutdown = errors.New("shutting down")
)

// NewService returns a service; call Start to schedule the stored jobs.
func NewService(st *store.SQLite, opts Options) *Service {
	if opts.Keep == 0 {
		opts.Keep = 100
	}
	if opts.MaxOutput == 0 {
		opts.MaxOutput = 256 << 10
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Service{
		store: st, opts: opts, log: opts.Logger, runners: map[string]runner.Runner{},
		timers: map[int64]*time.Timer{}, next: map[int64]time.Time{}, active: map[int64]*active{},
	}
	if !opts.NoLocal {
		s.runners["local"] = runner.Local{}
	}
	for _, r := range opts.Runners {
		s.runners[r.Kind()] = r
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// Start marks runs left over from a previous process as interrupted and
// schedules every enabled job. Runs missed while the program was down are
// not made up.
func (s *Service) Start(ctx context.Context) error {
	if err := s.store.InterruptRuns(ctx); err != nil {
		return err
	}
	jobs, err := s.store.Jobs(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	for _, j := range jobs {
		s.scheduleLocked(j)
	}
	return nil
}

// Close stops scheduling, cancels running jobs (recorded as interrupted)
// and waits for them to finish.
func (s *Service) Close() {
	s.mu.Lock()
	s.started = false
	for id, t := range s.timers {
		t.Stop()
		delete(s.timers, id)
	}
	for _, a := range s.active {
		a.cancel(errShutdown)
	}
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
}

func location(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	return time.LoadLocation(name)
}

// scheduleLocked (re)arms j's timer. Callers hold s.mu.
func (s *Service) scheduleLocked(j store.Job) {
	if t := s.timers[j.ID]; t != nil {
		t.Stop()
		delete(s.timers, j.ID)
	}
	delete(s.next, j.ID)
	if !j.Enabled || !s.started {
		return
	}
	sched, err := schedule.Parse(j.Schedule)
	if err != nil {
		s.log.Error("cron: unreadable schedule", "job", j.Name, "err", err)
		return
	}
	loc, err := location(j.Timezone)
	if err != nil {
		s.log.Error("cron: unknown timezone", "job", j.Name, "err", err)
		return
	}
	next := sched.Next(time.Now(), loc)
	if next.IsZero() {
		return
	}
	id := j.ID
	s.next[id] = next
	s.timers[id] = time.AfterFunc(time.Until(next), func() { s.fire(id) })
}

// fire runs a job at its scheduled time and arms the next one.
func (s *Service) fire(id int64) {
	j, err := s.store.Job(s.ctx, id)
	if err != nil {
		return // deleted meanwhile
	}
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.scheduleLocked(j)
	s.mu.Unlock()
	if !j.Enabled {
		return
	}
	if _, err := s.start(j, "schedule"); err != nil {
		if errors.Is(err, ErrRunning) {
			s.log.Warn("cron: skipped, the previous run is still going", "job", j.Name)
			return
		}
		s.log.Error("cron: could not start", "job", j.Name, "err", err)
	}
}

// start begins a run of j and returns it.
func (s *Service) start(j store.Job, trigger string) (store.Run, error) {
	r, ok := s.runners[runner.Kind(j.Target)]
	s.mu.Lock()
	if s.active[j.ID] != nil {
		s.mu.Unlock()
		return store.Run{}, ErrRunning
	}
	run := store.Run{JobID: j.ID, Trigger: trigger, Status: store.StatusRunning, StartedAt: time.Now()}
	if err := s.store.StartRun(s.ctx, &run); err != nil {
		s.mu.Unlock()
		return store.Run{}, err
	}
	ctx, cancel := context.WithCancelCause(s.ctx)
	a := &active{run: run, out: &tail{max: s.opts.MaxOutput}, cancel: cancel}
	s.active[j.ID] = a
	s.wg.Add(1)
	s.mu.Unlock()

	go func(run store.Run) { // a copy: the caller returns the original
		defer s.wg.Done()
		defer cancel(nil)
		runCtx := ctx
		if j.Timeout > 0 {
			var stop context.CancelFunc
			runCtx, stop = context.WithTimeoutCause(ctx, time.Duration(j.Timeout)*time.Second, context.DeadlineExceeded)
			defer stop()
		}
		code, err := -1, fmt.Errorf("no runner for target %q", j.Target)
		if ok {
			code, err = r.Run(runCtx, j.Target, runner.Command{Script: j.Command, Dir: j.Dir}, a.out)
		}
		run.ExitCode, run.FinishedAt, run.Output = code, time.Now(), a.out.String()
		switch cause := context.Cause(runCtx); {
		case errors.Is(cause, errShutdown):
			run.Status = store.StatusInterrupted
		case errors.Is(cause, errCanceled):
			run.Status = store.StatusCanceled
		case errors.Is(cause, context.DeadlineExceeded):
			run.Status, run.Error = store.StatusTimeout, fmt.Sprintf("timed out after %ds", j.Timeout)
		case err != nil:
			run.Status, run.Error = store.StatusFailed, err.Error()
		case code != 0:
			run.Status = store.StatusFailed
		default:
			run.Status = store.StatusSucceeded
		}
		if err := s.store.FinishRun(context.WithoutCancel(ctx), run, s.opts.Keep); err != nil {
			s.log.Error("cron: recording the run failed", "job", j.Name, "err", err)
		}
		s.mu.Lock()
		delete(s.active, j.ID)
		s.mu.Unlock()
		s.log.Info("cron: run finished", "job", j.Name, "trigger", trigger, "status", run.Status, "exit_code", code)
	}(run)
	return run, nil
}

// validate normalizes j and checks it.
func (s *Service) validate(ctx context.Context, j *store.Job) error {
	j.Name, j.Schedule, j.Timezone = strings.TrimSpace(j.Name), strings.TrimSpace(j.Schedule), strings.TrimSpace(j.Timezone)
	j.Target, j.Dir = strings.TrimSpace(j.Target), strings.TrimSpace(j.Dir)
	if j.Target == "" {
		j.Target = "local"
	}
	switch {
	case j.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalid)
	case strings.TrimSpace(j.Command) == "":
		return fmt.Errorf("%w: command required", ErrInvalid)
	case j.Timeout < 0:
		return fmt.Errorf("%w: timeout must not be negative", ErrInvalid)
	}
	if _, err := schedule.Parse(j.Schedule); err != nil {
		return fmt.Errorf("%w: schedule: %v", ErrInvalid, err)
	}
	if _, err := location(j.Timezone); err != nil {
		return fmt.Errorf("%w: timezone: %v", ErrInvalid, err)
	}
	r, ok := s.runners[runner.Kind(j.Target)]
	if !ok {
		return fmt.Errorf("%w: unknown target %q", ErrInvalid, j.Target)
	}
	// A target that went away (e.g. a deleted host) is refused; if the list
	// is unavailable, trust the caller.
	if targets, err := r.Targets(ctx); err == nil && !slices.ContainsFunc(targets, func(t runner.Target) bool { return t.ID == j.Target }) {
		return fmt.Errorf("%w: unknown target %q", ErrInvalid, j.Target)
	}
	return nil
}

func (s *Service) view(j store.Job, last map[int64]store.Run) JobView {
	v := JobView{Job: j}
	s.mu.Lock()
	v.NextRun = s.next[j.ID]
	if a := s.active[j.ID]; a != nil {
		run := a.run
		v.Running, v.LastRun = &run, &run
	}
	s.mu.Unlock()
	if r, ok := last[j.ID]; ok && v.LastRun == nil {
		v.LastRun = &r
	}
	return v
}

// Jobs returns every job with its state.
func (s *Service) Jobs(ctx context.Context) ([]JobView, error) {
	jobs, err := s.store.Jobs(ctx)
	if err != nil {
		return nil, err
	}
	last, err := s.store.LastRuns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]JobView, len(jobs))
	for i, j := range jobs {
		out[i] = s.view(j, last)
	}
	return out, nil
}

// Job returns one job with its state.
func (s *Service) Job(ctx context.Context, id int64) (JobView, error) {
	j, err := s.store.Job(ctx, id)
	if err != nil {
		return JobView{}, err
	}
	runs, err := s.store.Runs(ctx, id, 1)
	last := map[int64]store.Run{}
	if err == nil && len(runs) > 0 {
		last[id] = runs[0]
	}
	return s.view(j, last), nil
}

// CreateJob stores and schedules a job.
func (s *Service) CreateJob(ctx context.Context, j store.Job) (JobView, error) {
	if err := s.validate(ctx, &j); err != nil {
		return JobView{}, err
	}
	if err := s.store.CreateJob(ctx, &j); err != nil {
		return JobView{}, err
	}
	s.mu.Lock()
	s.scheduleLocked(j)
	s.mu.Unlock()
	return s.Job(ctx, j.ID)
}

// UpdateJob replaces a job and reschedules it. A run in progress finishes.
func (s *Service) UpdateJob(ctx context.Context, id int64, j store.Job) (JobView, error) {
	old, err := s.store.Job(ctx, id)
	if err != nil {
		return JobView{}, err
	}
	j.ID, j.CreatedAt = id, old.CreatedAt
	if err := s.validate(ctx, &j); err != nil {
		return JobView{}, err
	}
	if err := s.store.UpdateJob(ctx, &j); err != nil {
		return JobView{}, err
	}
	s.mu.Lock()
	s.scheduleLocked(j)
	s.mu.Unlock()
	return s.Job(ctx, id)
}

// SetEnabled turns a job's schedule on or off.
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) (JobView, error) {
	j, err := s.store.Job(ctx, id)
	if err != nil {
		return JobView{}, err
	}
	j.Enabled = enabled
	if err := s.store.UpdateJob(ctx, &j); err != nil {
		return JobView{}, err
	}
	s.mu.Lock()
	s.scheduleLocked(j)
	s.mu.Unlock()
	return s.Job(ctx, id)
}

// DeleteJob cancels a running run, unschedules the job and removes it with
// its history.
func (s *Service) DeleteJob(ctx context.Context, id int64) error {
	s.mu.Lock()
	if t := s.timers[id]; t != nil {
		t.Stop()
		delete(s.timers, id)
	}
	delete(s.next, id)
	if a := s.active[id]; a != nil {
		a.cancel(errCanceled)
	}
	s.mu.Unlock()
	return s.store.DeleteJob(ctx, id)
}

// RunNow starts a job immediately, whether or not it is enabled.
func (s *Service) RunNow(ctx context.Context, id int64) (store.Run, error) {
	j, err := s.store.Job(ctx, id)
	if err != nil {
		return store.Run{}, err
	}
	return s.start(j, "manual")
}

// Run returns one run; a run in progress carries its output so far.
func (s *Service) Run(ctx context.Context, id int64) (store.Run, error) {
	s.mu.Lock()
	for _, a := range s.active {
		if a.run.ID == id {
			run := a.run
			run.Output = a.out.String()
			s.mu.Unlock()
			return run, nil
		}
	}
	s.mu.Unlock()
	return s.store.Run(ctx, id)
}

// Runs returns a job's latest runs, newest first.
func (s *Service) Runs(ctx context.Context, jobID int64) ([]store.Run, error) {
	if _, err := s.store.Job(ctx, jobID); err != nil {
		return nil, err
	}
	return s.store.Runs(ctx, jobID, s.opts.Keep)
}

// Cancel stops a run in progress.
func (s *Service) Cancel(ctx context.Context, runID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.active {
		if a.run.ID == runID {
			a.cancel(errCanceled)
			return nil
		}
	}
	if _, err := s.store.Run(ctx, runID); err != nil {
		return err
	}
	return fmt.Errorf("%w: the run has finished", ErrInvalid)
}

// Targets lists where jobs can run.
func (s *Service) Targets(ctx context.Context) ([]runner.Target, error) {
	kinds := make([]string, 0, len(s.runners))
	for k := range s.runners {
		kinds = append(kinds, k)
	}
	slices.SortFunc(kinds, func(a, b string) int { // "local" first, then by kind
		if (a == "local") != (b == "local") {
			if a == "local" {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	out := []runner.Target{}
	for _, k := range kinds {
		targets, err := s.runners[k].Targets(ctx)
		if err != nil {
			s.log.Warn("cron: listing targets failed", "kind", k, "err", err)
			continue
		}
		out = append(out, targets...)
	}
	return out, nil
}

// Preview returns the next n times expr fires in timezone.
func Preview(expr, timezone string, n int) ([]time.Time, error) {
	sched, err := schedule.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	loc, err := location(strings.TrimSpace(timezone))
	if err != nil {
		return nil, fmt.Errorf("%w: timezone: %v", ErrInvalid, err)
	}
	return schedule.NextN(sched, time.Now(), loc, n), nil
}

// Summary counts jobs and their state.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	jobs, err := s.Jobs(ctx)
	var sum Summary
	for _, j := range jobs {
		sum.Jobs++
		if j.Enabled {
			sum.Enabled++
		}
		switch {
		case j.Running != nil:
			sum.Running++
		case j.LastRun != nil && j.LastRun.Status != store.StatusSucceeded:
			sum.Failing++
		}
	}
	return sum, err
}

// tail keeps the last max bytes written to it.
type tail struct {
	mu        sync.Mutex
	max       int
	buf       []byte
	truncated bool
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
		t.truncated = true
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.truncated {
		return "…(earlier output omitted)\n" + string(t.buf)
	}
	return string(t.buf)
}
