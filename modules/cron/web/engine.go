package web

import (
	"context"
	"errors"
	"fmt"
	cronmodule "github.com/lsongdev/ohmyserver/modules/cron"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type CommandRunner func(context.Context, Job) (string, error)

type scheduledJob struct {
	job   Job
	timer *time.Timer
}

// Engine owns cron scheduling, process execution, execution history, and
// repository coordination. Call Close before closing the repository's DB.
type Engine struct {
	repository *Repository
	ctx        context.Context
	cancel     context.CancelFunc
	runner     CommandRunner

	mu      sync.Mutex
	jobs    map[int64]*scheduledJob
	workers sync.WaitGroup
	started bool
	closed  bool
}

func NewEngine(ctx context.Context, repository *Repository, runner CommandRunner) *Engine {
	engineCtx, cancel := context.WithCancel(ctx)
	if runner == nil {
		runner = executeCommand
	}
	return &Engine{repository: repository, ctx: engineCtx, cancel: cancel, runner: runner, jobs: make(map[int64]*scheduledJob)}
}

func (e *Engine) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := e.repository.Initialize(ctx); err != nil {
		return err
	}
	if err := e.repository.RecoverInterruptedExecutions(ctx); err != nil {
		return err
	}
	jobs, err := e.repository.Enabled(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("cron engine is closed")
	}
	if e.started {
		return errors.New("cron engine is already started")
	}
	e.started = true
	for i := range jobs {
		e.scheduleLocked(jobs[i])
	}
	return nil
}

func (e *Engine) Close() error {
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		e.cancel()
		for id, item := range e.jobs {
			if item.timer != nil {
				item.timer.Stop()
			}
			delete(e.jobs, id)
		}
	}
	e.mu.Unlock()
	e.workers.Wait()
	return nil
}

func (e *Engine) CronJobs(ctx context.Context) ([]Job, error) { return e.repository.List(ctx) }
func (e *Engine) CronJob(ctx context.Context, id int64) (Job, error) {
	return e.repository.Get(ctx, id)
}
func (e *Engine) CreateCronJobAPI(ctx context.Context, in Input) (Job, error) {
	if err := validateInput(in); err != nil {
		return Job{}, errors.Join(ErrInvalidInput, err)
	}
	job, err := e.repository.Create(ctx, in)
	if err != nil {
		return Job{}, err
	}
	e.mu.Lock()
	if !e.closed {
		e.scheduleLocked(job)
	}
	e.mu.Unlock()
	return job, nil
}
func (e *Engine) UpdateCronJobAPI(ctx context.Context, id int64, in Input) error {
	if err := validateInput(in); err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	if _, err := e.repository.Get(ctx, id); err != nil {
		return err
	}
	if err := e.repository.Update(ctx, id, in); err != nil {
		return err
	}
	job, err := e.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if job.Enabled && !e.closed {
		e.scheduleLocked(job)
	} else {
		e.unscheduleLocked(id)
	}
	return nil
}
func (e *Engine) DeleteCronJobAPI(ctx context.Context, id int64) error {
	if _, err := e.repository.Get(ctx, id); err != nil {
		return err
	}
	if err := e.repository.Delete(ctx, id); err != nil {
		return err
	}
	e.mu.Lock()
	e.unscheduleLocked(id)
	e.mu.Unlock()
	return nil
}
func (e *Engine) SetCronJobEnabled(ctx context.Context, id int64, enabled bool) error {
	if _, err := e.repository.Get(ctx, id); err != nil {
		return err
	}
	if err := e.repository.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	job, err := e.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if enabled && !e.closed {
		e.scheduleLocked(job)
	} else {
		e.unscheduleLocked(id)
	}
	return nil
}
func (e *Engine) RunCronJobNow(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, err := e.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if e.closed || e.ctx.Err() != nil {
		e.mu.Unlock()
		return errors.New("cron engine is closed")
	}
	e.workers.Add(1)
	e.mu.Unlock()
	go e.execute(job, nil)
	return nil
}
func (e *Engine) CronJobExecutions(ctx context.Context, id int64, limit int) ([]Execution, error) {
	if _, err := e.repository.Get(ctx, id); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	return e.repository.Executions(ctx, id, limit)
}

func (e *Engine) scheduleLocked(job Job) {
	e.unscheduleLocked(job.ID)
	if !job.Enabled || e.closed || e.ctx.Err() != nil {
		return
	}
	next, err := nextRun(job)
	if err != nil || next == nil {
		return
	}
	item := &scheduledJob{job: job}
	e.jobs[job.ID] = item
	item.timer = time.AfterFunc(time.Until(*next), func() { e.runScheduled(item) })
}

func (e *Engine) unscheduleLocked(id int64) {
	if item := e.jobs[id]; item != nil {
		if item.timer != nil {
			item.timer.Stop()
		}
		delete(e.jobs, id)
	}
}

func (e *Engine) runScheduled(item *scheduledJob) {
	e.mu.Lock()
	if e.closed || e.ctx.Err() != nil || e.jobs[item.job.ID] != item {
		e.mu.Unlock()
		return
	}
	e.workers.Add(1)
	e.mu.Unlock()
	e.execute(item.job, item)
}

func (e *Engine) execute(job Job, scheduled *scheduledJob) {
	defer e.workers.Done()
	persistCtx := context.WithoutCancel(e.ctx)
	execution, err := e.repository.StartExecution(persistCtx, job.ID)
	if err != nil {
		return
	}
	ctx := e.ctx
	var cancel context.CancelFunc
	if job.Timeout > 0 {
		ctx, cancel = context.WithTimeout(e.ctx, time.Duration(job.Timeout)*time.Second)
		defer cancel()
	}
	output, runErr := e.runner(ctx, job)
	status, errorMessage, exitCode := "success", "", 0
	if runErr != nil {
		status, exitCode = "failed", -1
		errorMessage = runErr.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = "timeout"
			errorMessage = fmt.Sprintf("Command timed out after %d seconds", job.Timeout)
		}
	}
	_ = e.repository.CompleteExecution(persistCtx, execution.ID, status, output, errorMessage, exitCode)
	var next *time.Time
	if job.Type == "cron" {
		next, _ = cronmodule.ParseCronExpression(job.Schedule)
	} else if job.Type == "once" {
		_ = e.repository.SetEnabled(persistCtx, job.ID, false)
	}
	_ = e.repository.UpdateRunTimes(persistCtx, job.ID, time.Now(), next)

	updated, err := e.repository.Get(persistCtx, job.ID)
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.ctx.Err() != nil || (scheduled != nil && e.jobs[job.ID] != scheduled) {
		return
	}
	if updated.Enabled {
		e.scheduleLocked(updated)
	} else {
		e.unscheduleLocked(job.ID)
	}
}

func nextRun(job Job) (*time.Time, error) {
	if job.Type == "cron" {
		return cronmodule.ParseCronExpression(job.Schedule)
	}
	if job.Type == "once" {
		runAt, err := time.Parse(time.RFC3339, job.Schedule)
		if err != nil {
			return nil, err
		}
		if runAt.After(time.Now()) {
			return &runAt, nil
		}
		return nil, nil
	}
	return nil, ErrInvalidInput
}

func validateInput(in Input) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Schedule) == "" || strings.TrimSpace(in.Command) == "" || in.Timeout < 0 {
		return ErrInvalidInput
	}
	if _, err := nextRun(Job{Type: in.Type, Schedule: in.Schedule}); err != nil {
		return err
	}
	if in.Type == "once" {
		when, _ := time.Parse(time.RFC3339, in.Schedule)
		if !when.After(time.Now()) {
			return errors.New("one-time schedule must be in the future")
		}
	}
	return nil
}

func executeCommand(ctx context.Context, job Job) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
	if job.WorkingDir != "" {
		cmd.Dir = job.WorkingDir
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}
