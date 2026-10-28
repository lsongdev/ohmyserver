// Package api is the package manager's service and JSON API. Reading
// (installed packages, updates, search) runs the package manager directly;
// changes (install, remove, upgrade, refreshing metadata, installing
// Docker) run as jobs, one at a time, whose output the UI follows.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/pkgmgr/system"
)

// Options configures the service.
type Options struct {
	Logger *slog.Logger
	// Manager overrides the detected package manager (tests).
	Manager system.Manager
	// Runner runs commands; nil runs them on this host.
	Runner system.Runner
	// Root is the file system root, "/" unless testing.
	Root string
}

// Errors the handler maps to statuses.
var (
	ErrBusy     = errors.New("busy")
	ErrNotFound = errors.New("not found")
)

// Service manages the system's packages.
type Service struct {
	manager system.Manager // nil when unsupported
	os      system.OS
	docker  *system.Docker
	logger  *slog.Logger

	mu      sync.Mutex
	jobs    []*Job // newest last, at most keepJobs
	running *Job
	nextID  int64
	wg      sync.WaitGroup
}

// keepJobs is how many jobs are remembered; keepLines how much output each
// keeps.
const (
	keepJobs  = 50
	keepLines = 5000
)

// NewService detects the package manager.
func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	run := opts.Runner
	if run == nil {
		run = system.Exec{}
	}
	root := opts.Root
	if root == "" {
		root = "/"
	}
	manager := opts.Manager
	if manager == nil {
		var err error
		if manager, err = system.Detect(run); err != nil {
			logger.Warn("packages: " + err.Error())
		}
	}
	osInfo := system.ReadOS(root)
	return &Service{
		manager: manager, os: osInfo, logger: logger,
		docker: &system.Docker{Manager: manager, OS: osInfo, Run: run, Root: root},
	}
}

// Info describes the system and the job running, if any.
type Info struct {
	Manager string    `json:"manager,omitempty"` // empty when unsupported
	OS      system.OS `json:"os"`
	Running *Job      `json:"running,omitempty"`
}

func (s *Service) Info() Info {
	info := Info{OS: s.os, Running: s.Running()}
	if s.manager != nil {
		info.Manager = s.manager.Name()
	}
	return info
}

func (s *Service) need() (system.Manager, error) {
	if s.manager == nil {
		return nil, system.ErrUnsupported
	}
	return s.manager, nil
}

func (s *Service) Installed(ctx context.Context) ([]system.Package, error) {
	m, err := s.need()
	if err != nil {
		return nil, err
	}
	return m.Installed(ctx)
}

// Updates lists available updates with the versions installed now.
func (s *Service) Updates(ctx context.Context) ([]system.Update, error) {
	m, err := s.need()
	if err != nil {
		return nil, err
	}
	updates, err := m.Updates(ctx)
	if err != nil {
		return nil, err
	}
	installed, err := m.Installed(ctx)
	if err != nil {
		return nil, err
	}
	current := map[string]string{}
	for _, p := range installed {
		current[p.Name+"."+p.Arch] = p.Version
		if _, ok := current[p.Name]; !ok {
			current[p.Name] = p.Version
		}
	}
	for i, u := range updates {
		if v, ok := current[u.Name+"."+u.Arch]; ok {
			updates[i].Current = v
		} else {
			updates[i].Current = current[u.Name]
		}
	}
	return updates, nil
}

// Search finds packages and marks those installed.
func (s *Service) Search(ctx context.Context, query string) ([]system.Package, error) {
	m, err := s.need()
	if err != nil {
		return nil, err
	}
	results, err := m.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	installed, err := m.Installed(ctx)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, p := range installed {
		have[p.Name] = true
	}
	for i := range results {
		results[i].Installed = results[i].Installed || have[results[i].Name]
	}
	return results, nil
}

func (s *Service) PackageInfo(ctx context.Context, name string) (string, error) {
	m, err := s.need()
	if err != nil {
		return "", err
	}
	return m.Info(ctx, name)
}

func (s *Service) DockerStatus(ctx context.Context) system.DockerStatus {
	return s.docker.Status(ctx)
}

// Job is a change to the system and its output.
type Job struct {
	ID         int64      `json:"id"`
	Action     string     `json:"action"`
	Packages   []string   `json:"packages,omitempty"`
	Title      string     `json:"title"`
	Status     string     `json:"status"` // running, succeeded, failed
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Output     []string   `json:"output,omitempty"`
}

// JobRequest starts a job. Action is install, remove or upgrade (with
// packages; upgrade without packages upgrades everything), refresh, or
// docker (with a source).
type JobRequest struct {
	Action   string   `json:"action"`
	Packages []string `json:"packages"`
	Source   string   `json:"source"`
}

// Start validates req and runs it as a job unless another one is running.
func (s *Service) Start(req JobRequest) (*Job, error) {
	m, err := s.need()
	if err != nil {
		return nil, err
	}
	if err := system.ValidateNames(req.Packages); err != nil {
		return nil, err
	}
	names := strings.Join(req.Packages, " ")
	var title string
	var run func(ctx context.Context, out func(string)) error
	switch req.Action {
	case "install", "remove":
		if len(req.Packages) == 0 {
			return nil, fmt.Errorf("%w: name the packages to %s", system.ErrInvalid, req.Action)
		}
		title = map[string]string{"install": "Install ", "remove": "Remove "}[req.Action] + names
		run = func(ctx context.Context, out func(string)) error {
			if req.Action == "install" {
				return m.Install(ctx, req.Packages, out)
			}
			return m.Remove(ctx, req.Packages, out)
		}
	case "upgrade":
		if len(req.Packages) > 0 && m.Name() == "pacman" {
			return nil, fmt.Errorf("%w upgrade: %v", system.ErrInvalid, system.ErrPartialUpgrade)
		}
		title = "Upgrade " + names
		if len(req.Packages) == 0 {
			title = "Upgrade all packages"
		}
		run = func(ctx context.Context, out func(string)) error { return m.Upgrade(ctx, req.Packages, out) }
	case "refresh":
		title = "Refresh package lists"
		run = m.Refresh
	case "docker":
		title = "Install Docker"
		run = func(ctx context.Context, out func(string)) error { return s.docker.Install(ctx, req.Source, out) }
	default:
		return nil, fmt.Errorf("%w action %q", system.ErrInvalid, req.Action)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil {
		return nil, fmt.Errorf("%w: %q is still running", ErrBusy, s.running.Title)
	}
	s.nextID++
	job := &Job{ID: s.nextID, Action: req.Action, Packages: req.Packages, Title: title, Status: "running", StartedAt: time.Now().UTC()}
	s.running = job
	if s.jobs = append(s.jobs, job); len(s.jobs) > keepJobs {
		s.jobs = s.jobs[len(s.jobs)-keepJobs:]
	}
	s.logger.Info("packages: job started", "job", job.ID, "title", title)
	s.wg.Go(func() {
		// A package transaction is not interrupted halfway: the job keeps
		// running even if the request that started it goes away.
		err := run(context.Background(), func(line string) {
			s.mu.Lock()
			if job.Output = append(job.Output, line); len(job.Output) > keepLines {
				job.Output = job.Output[len(job.Output)-keepLines:]
			}
			s.mu.Unlock()
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		now := time.Now().UTC()
		job.FinishedAt, job.Status = &now, "succeeded"
		if err != nil {
			job.Status, job.Error = "failed", err.Error()
		}
		s.running = nil
		s.logger.Info("packages: job finished", "job", job.ID, "status", job.Status)
	})
	return job.copy(false), nil
}

// copy snapshots a job, with its output or without; the caller holds s.mu.
func (j *Job) copy(output bool) *Job {
	c := *j
	c.Output = nil
	if output {
		c.Output = append([]string{}, j.Output...)
	}
	return &c
}

// Jobs lists jobs, newest first, without their output.
func (s *Service) Jobs() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*Job, 0, len(s.jobs))
	for i := len(s.jobs) - 1; i >= 0; i-- {
		result = append(result, s.jobs[i].copy(false))
	}
	return result
}

// Job returns a job with its output.
func (s *Service) Job(id int64) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.ID == id {
			return j.copy(true), nil
		}
	}
	return nil, fmt.Errorf("%w: job %d", ErrNotFound, id)
}

// Running returns the job running now, or nil.
func (s *Service) Running() *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running == nil {
		return nil
	}
	return s.running.copy(false)
}

// Wait waits for the running job to finish.
func (s *Service) Wait() { s.wg.Wait() }
