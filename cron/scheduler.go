package cron

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Job represents a schedulable job
type Job interface {
	GetID() int64
	GetName() string
	GetType() string
	GetSchedule() string
	GetCommand() string
	GetWorkingDir() string
	GetTimeout() int
	IsEnabled() bool
}

// JobExecutor is a function that executes a job
type JobExecutor func(ctx context.Context, job Job) error

// Scheduler manages scheduled task execution
type Scheduler struct {
	jobs       map[int64]*scheduledJob
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	executor   JobExecutor
	onComplete func(job Job, executionID int64, status string, output string, errorMsg string, exitCode int)
}

type scheduledJob struct {
	job      Job
	timer    *time.Timer
	nextRun  time.Time
	stopChan chan struct{}
}

// NewScheduler creates a new cron scheduler
func NewScheduler(executor JobExecutor) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		jobs:     make(map[int64]*scheduledJob),
		ctx:      ctx,
		cancel:   cancel,
		executor: executor,
	}
}

// SetOnComplete sets the callback function for job completion
func (s *Scheduler) SetOnComplete(fn func(job Job, executionID int64, status string, output string, errorMsg string, exitCode int)) {
	s.onComplete = fn
}

// Start starts the cron scheduler
func (s *Scheduler) Start(jobs []Job) error {
	for _, job := range jobs {
		if err := s.ScheduleJob(job); err != nil {
			fmt.Printf("Failed to schedule job %s: %v\n", job.GetName(), err)
		}
	}

	// Start the scheduler loop
	s.wg.Add(1)
	go s.run()

	return nil
}

// Stop stops the cron scheduler
func (s *Scheduler) Stop() {
	s.cancel()

	// Stop all job timers
	for _, sj := range s.jobs {
		close(sj.stopChan)
		if sj.timer != nil {
			sj.timer.Stop()
		}
	}

	s.wg.Wait()
}

// ScheduleJob schedules a single cron job
func (s *Scheduler) ScheduleJob(job Job) error {

	// Remove existing job if present
	if existing, ok := s.jobs[job.GetID()]; ok {
		close(existing.stopChan)
		if existing.timer != nil {
			existing.timer.Stop()
		}
	}

	// Calculate next run time
	var nextRun time.Time
	if job.GetType() == "cron" {
		t, err := ParseCronExpression(job.GetSchedule())
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		if t == nil {
			return fmt.Errorf("could not calculate next run time")
		}
		nextRun = *t
	} else if job.GetType() == "once" {
		t, err := time.Parse(time.RFC3339, job.GetSchedule())
		if err != nil {
			return fmt.Errorf("invalid time format: %w", err)
		}
		if !t.After(time.Now()) {
			// Already passed, skip this job
			return nil
		}
		nextRun = t
	}

	// Create scheduled job
	sj := &scheduledJob{
		job:      job,
		nextRun:  nextRun,
		stopChan: make(chan struct{}),
	}

	// Set timer
	duration := time.Until(nextRun)
	sj.timer = time.AfterFunc(duration, func() {
		s.executeJob(job)
	})

	s.jobs[job.GetID()] = sj
	fmt.Printf("Scheduled job %s (type: %s) to run at %s\n", job.GetName(), job.GetType(), nextRun.Format("2006-01-02 15:04:05"))

	return nil
}

// UnscheduleJob removes a job from the scheduler
func (s *Scheduler) UnscheduleJob(jobID int64) {

	if sj, ok := s.jobs[jobID]; ok {
		close(sj.stopChan)
		if sj.timer != nil {
			sj.timer.Stop()
		}
		delete(s.jobs, jobID)
		fmt.Printf("Unscheduled job %d\n", jobID)
	}
}

// RescheduleJob reschedules a job (called after job execution)
func (s *Scheduler) RescheduleJob(jobID int64) {

	sj, ok := s.jobs[jobID]
	if !ok {
		return
	}

	// Stop existing timer first to prevent duplicate executions
	if sj.timer != nil {
		sj.timer.Stop()
	}

	// Recalculate next run time based on current job data
	var nextRun time.Time
	if sj.job.GetType() == "cron" {
		t, err := ParseCronExpression(sj.job.GetSchedule())
		if err == nil && t != nil {
			nextRun = *t
		}
	} else if sj.job.GetType() == "once" {
		t, err := time.Parse(time.RFC3339, sj.job.GetSchedule())
		if err == nil && t.After(time.Now()) {
			nextRun = t
		}
	}

	if !nextRun.IsZero() {
		sj.nextRun = nextRun
		duration := time.Until(nextRun)
		sj.timer = time.AfterFunc(duration, func() {
			s.executeJob(sj.job)
		})
		fmt.Printf("Rescheduled job %s to run at %s\n", sj.job.GetName(), nextRun.Format("2006-01-02 15:04:05"))
	}
}

// run is the main scheduler loop that periodically checks for jobs to reschedule
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRescheduleJobs()
		}
	}
}

// checkAndRescheduleJobs checks all cron jobs and reschedules them if needed
func (s *Scheduler) checkAndRescheduleJobs() {

	jobIDs := make([]int64, 0, len(s.jobs))
	for id := range s.jobs {
		jobIDs = append(jobIDs, id)
	}

	// Note: We don't need to reschedule jobs here because:
	// 1. Jobs are rescheduled automatically after execution via RescheduleJob
	// 2. The scheduler loop is mainly for periodic cleanup
	// This method is kept for potential future use (e.g., detecting missed jobs)
	_ = jobIDs
}

// executeJob executes a single cron job
func (s *Scheduler) executeJob(job Job) {
	fmt.Printf("Executing job %s: %s\n", job.GetName(), job.GetCommand())

	// Create a context for execution
	ctx := context.Background()
	if job.GetTimeout() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(job.GetTimeout())*time.Second)
		defer cancel()
	}

	// Execute the job
	err := s.executor(ctx, job)

	var status string
	var errorMsg string
	var exitCode int

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			status = "timeout"
			errorMsg = fmt.Sprintf("Command timed out after %d seconds", job.GetTimeout())
		} else {
			status = "failed"
			errorMsg = err.Error()
		}
		exitCode = -1
	} else {
		status = "success"
		exitCode = 0
	}

	fmt.Printf("Job %s completed with status: %s\n", job.GetName(), status)

	// Call completion callback if set
	if s.onComplete != nil {
		s.onComplete(job, 0, status, "", errorMsg, exitCode)
	}

	// Reschedule for next run
	s.RescheduleJob(job.GetID())
}

// GetJobStatus returns the status of a scheduled job
func (s *Scheduler) GetJobStatus(jobID int64) (nextRun *time.Time, scheduled bool) {

	if sj, ok := s.jobs[jobID]; ok {
		return &sj.nextRun, true
	}
	return nil, false
}

// RunJobNow executes a job immediately
func (s *Scheduler) RunJobNow(job Job) error {
	// Execute in goroutine
	go s.executeJob(job)
	return nil
}

// GetScheduledJobs returns all currently scheduled job IDs
func (s *Scheduler) GetScheduledJobs() []int64 {

	ids := make([]int64, 0, len(s.jobs))
	for id := range s.jobs {
		ids = append(ids, id)
	}
	return ids
}
