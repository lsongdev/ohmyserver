package cron

import "time"

type Job struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Schedule   string     `json:"schedule"`
	Command    string     `json:"command"`
	WorkingDir string     `json:"working_dir"`
	Timeout    int        `json:"timeout"`
	Enabled    bool       `json:"enabled"`
	LastRun    *time.Time `json:"last_run"`
	NextRun    *time.Time `json:"next_run"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (j *Job) GetID() int64          { return j.ID }
func (j *Job) GetName() string       { return j.Name }
func (j *Job) GetType() string       { return j.Type }
func (j *Job) GetSchedule() string   { return j.Schedule }
func (j *Job) GetCommand() string    { return j.Command }
func (j *Job) GetWorkingDir() string { return j.WorkingDir }
func (j *Job) GetTimeout() int       { return j.Timeout }
func (j *Job) IsEnabled() bool       { return j.Enabled }

type Execution struct {
	ID          int64      `json:"id"`
	JobID       int64      `json:"job_id"`
	Status      string     `json:"status"`
	Output      string     `json:"output"`
	Error       string     `json:"error"`
	ExitCode    int        `json:"exit_code"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Input struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Schedule   string `json:"schedule"`
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir"`
	Timeout    int    `json:"timeout"`
}
