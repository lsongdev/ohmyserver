package server

import (
	"context"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	"github.com/lsongdev/ohmyserver/cron"
)

// CronJobView handles the cron job list page
func (s *Server) CronJobView(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.GetAllCronJobs()
	if err != nil {
		s.Error(w, err)
		return
	}

	jobList := make([]H, len(jobs))
	for i, job := range jobs {
		nextRunStr := "-"
		if job.NextRun != nil {
			nextRunStr = job.NextRun.Format("2006-01-02 15:04:05")
		}
		lastRunStr := "-"
		if job.LastRun != nil {
			lastRunStr = job.LastRun.Format("2006-01-02 15:04:05")
		}

		jobList[i] = H{
			"ID":        job.ID,
			"Name":      job.Name,
			"Type":      job.Type,
			"Schedule":  job.Schedule,
			"Command":   job.Command,
			"Enabled":   job.Enabled,
			"LastRun":   lastRunStr,
			"NextRun":   nextRunStr,
			"CreatedAt": job.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	s.Render(w, "cron/index", H{
		"jobs": jobList,
	})
}

// CronJobNewHandler handles creating a new cron job
func (s *Server) CronJobNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "cron/new", H{})
		return
	}

	err := r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := r.FormValue("name")
	jobType := r.FormValue("type")
	schedule := r.FormValue("schedule")
	command := r.FormValue("command")
	workingDir := r.FormValue("working_dir")
	timeoutStr := r.FormValue("timeout")

	if name == "" || jobType == "" || schedule == "" || command == "" {
		s.Error(w, &Error{Message: "Name, type, schedule, and command are required"})
		return
	}

	if jobType != "cron" && jobType != "once" {
		s.Error(w, &Error{Message: "Type must be cron or once"})
		return
	}

	timeout := 0
	if timeoutStr != "" {
		timeout, err = strconv.Atoi(timeoutStr)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid timeout value"})
			return
		}
	}

	// Validate schedule based on type
	if jobType == "once" {
		_, err := time.Parse(time.RFC3339, schedule)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid time format. Use RFC3339 format (e.g., 2026-04-01T12:00:00+08:00)"})
			return
		}
	}

	_, err = s.CreateCronJob(name, jobType, schedule, command, workingDir, timeout)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

// CronJobEditHandler handles editing a cron job
func (s *Server) CronJobEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		job, err := s.GetCronJob(id)
		if err != nil {
			s.Error(w, err)
			return
		}

		s.Render(w, "cron/edit", H{
			"job": job,
		})
		return
	}

	err = r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := r.FormValue("name")
	jobType := r.FormValue("type")
	schedule := r.FormValue("schedule")
	command := r.FormValue("command")
	workingDir := r.FormValue("working_dir")
	timeoutStr := r.FormValue("timeout")

	if name == "" || jobType == "" || schedule == "" || command == "" {
		s.Error(w, &Error{Message: "Name, type, schedule, and command are required"})
		return
	}

	if jobType != "cron" && jobType != "once" {
		s.Error(w, &Error{Message: "Type must be cron or once"})
		return
	}

	timeout := 0
	if timeoutStr != "" {
		timeout, err = strconv.Atoi(timeoutStr)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid timeout value"})
			return
		}
	}

	err = s.UpdateCronJob(id, name, jobType, schedule, command, workingDir, timeout)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Reschedule the job
	if s.cronScheduler != nil {
		job, err := s.GetCronJob(id)
		if err == nil {
			s.cronScheduler.ScheduleJob(job)
		}
	}

	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

// CronJobDeleteHandler handles deleting a cron job
func (s *Server) CronJobDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Unschedule first
	if s.cronScheduler != nil {
		s.cronScheduler.UnscheduleJob(id)
	}

	err = s.DeleteCronJob(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

// CronJobDetailHandler handles the cron job detail page
func (s *Server) CronJobDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	job, err := s.GetCronJob(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Get recent executions
	executions, err := s.GetCronJobExecutions(id, 20)
	if err != nil {
		s.Error(w, err)
		return
	}

	execList := make([]H, len(executions))
	for i, exec := range executions {
		completedAt := "-"
		if exec.CompletedAt != nil {
			completedAt = exec.CompletedAt.Format("2006-01-02 15:04:05")
		}

		execList[i] = H{
			"ID":          exec.ID,
			"Status":      exec.Status,
			"Output":      exec.Output,
			"Error":       exec.Error,
			"ExitCode":    exec.ExitCode,
			"StartedAt":   exec.StartedAt.Format("2006-01-02 15:04:05"),
			"CompletedAt": completedAt,
		}
	}

	s.Render(w, "cron/detail", H{
		"job":        job,
		"executions": execList,
	})
}

// CronJobToggleHandler handles enabling/disabling a cron job
func (s *Server) CronJobToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	action := r.URL.Query().Get("action")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	switch action {
	case "enable":
		if err := s.EnableCronJob(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Schedule the job
		if s.cronScheduler != nil {
			job, err := s.GetCronJob(id)
			if err == nil {
				s.cronScheduler.ScheduleJob(job)
			}
		}
	case "disable":
		if err := s.DisableCronJob(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Unschedule the job
		if s.cronScheduler != nil {
			s.cronScheduler.UnscheduleJob(id)
		}
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/cron", http.StatusSeeOther)
}

// CronJobRunNowHandler handles running a job immediately
func (s *Server) CronJobRunNowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if s.cronScheduler == nil {
		http.Error(w, "Scheduler not available", http.StatusInternalServerError)
		return
	}

	job, err := s.GetCronJob(id)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if err := s.cronScheduler.RunJobNow(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cron/"+idStr, http.StatusSeeOther)
}

// executeCronJob executes a cron job
func (s *Server) executeCronJob(ctx context.Context, job cron.Job) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", job.GetCommand())
	if job.GetWorkingDir() != "" {
		cmd.Dir = job.GetWorkingDir()
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	// Store output in context for later retrieval
	// Note: For a more complete implementation, we would store this in the database
	_ = output
	return nil
}

// onCronJobComplete handles cron job completion
func (s *Server) onCronJobComplete(job cron.Job, executionID int64, status string, output string, errorMsg string, exitCode int) {
	now := time.Now()

	// Calculate next run time
	var nextRun *time.Time
	if job.GetType() == "cron" {
		t, err := cron.ParseCronExpression(job.GetSchedule())
		if err == nil {
			nextRun = t
		}
	}

	// Update both last run and next run times
	if err := s.UpdateCronJobRunTimes(job.GetID(), now, nextRun); err != nil {
		log.Printf("Failed to update run times for job %s: %v\n", job.GetName(), err)
	}

	// Create execution record
	execution, err := s.CreateCronJobExecution(job.GetID())
	if err != nil {
		log.Printf("Failed to create execution record for job %s: %v\n", job.GetName(), err)
		return
	}

	// Update execution record with results
	if err := s.UpdateCronJobExecution(execution.ID, status, output, errorMsg, exitCode); err != nil {
		log.Printf("Failed to update execution record: %v\n", err)
	}
}

func init() {
	RegisterInitHook(func(s *Server) error {
		s.cronScheduler = cron.NewScheduler(s.executeCronJob)
		s.cronScheduler.SetOnComplete(s.onCronJobComplete)
		enabledJobs, err := s.GetEnabledCronJobs()
		if err != nil {
			log.Printf("Warning: Failed to load cron jobs: %v\n", err)
			return nil
		}
		jobs := make([]cron.Job, len(enabledJobs))
		for i := range enabledJobs {
			jobs[i] = &enabledJobs[i]
		}
		if err := s.cronScheduler.Start(jobs); err != nil {
			log.Printf("Warning: Failed to start cron scheduler: %v\n", err)
		}
		return nil
	})
	RegisterCloseHook(func(s *Server) error {
		if s.cronScheduler != nil {
			s.cronScheduler.Stop()
		}
		return nil
	})
}
