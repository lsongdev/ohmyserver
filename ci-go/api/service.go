// Package api queues and runs builds, keeps their history and logs, takes
// webhooks from git hosts, and serves it all as a JSON API with relative
// routes.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ci-go/pipeline"
	"github.com/lsongdev/ci-go/store"
)

// ErrInvalid marks invalid input.
var ErrInvalid = errors.New("invalid request")

// Options configures the service; zero values select the defaults.
type Options struct {
	Pipeline pipeline.Options
	Workers  int // builds run at once (1)
	Keep     int // builds kept per repository (50)
	MaxLog   int // bytes of log kept per step; the end is kept (1 MiB)
	Logger   *slog.Logger
}

// Cleaner removes what crashed builds left behind; docker.Client
// implements it.
type Cleaner interface {
	RemoveLabeled(ctx context.Context, label string) error
}

// RepoView is a repository as the API shows it: secrets only by name.
type RepoView struct {
	store.Repo
	HasToken      bool         `json:"has_token"`
	SecretNames   []string     `json:"secret_names"`
	WebhookSecret string       `json:"webhook_secret,omitempty"` // in single-repository views
	LastBuild     *store.Build `json:"last_build,omitempty"`
}

// Summary counts repositories and builds for an overview.
type Summary struct {
	Repos   int `json:"repos"`
	Running int `json:"running"`
	Pending int `json:"pending"`
	Failing int `json:"failing"` // repositories whose latest build did not succeed
}

// Service runs builds.
type Service struct {
	store *store.SQLite
	rt    pipeline.Runtime
	opts  Options
	log   *slog.Logger
	wake  chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	active map[int64]*activeBuild
}

type activeBuild struct {
	ctx    context.Context // ends when the build is cancelled
	cancel context.CancelCauseFunc
	logs   map[int]*logBuffer
}

var errCanceled = errors.New("canceled")

// NewService returns a service; call Start to run queued builds.
func NewService(st *store.SQLite, rt pipeline.Runtime, opts Options) *Service {
	if opts.Workers == 0 {
		opts.Workers = 1
	}
	if opts.Keep == 0 {
		opts.Keep = 50
	}
	if opts.MaxLog == 0 {
		opts.MaxLog = 1 << 20
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Service{store: st, rt: rt, opts: opts, log: opts.Logger, wake: make(chan struct{}, 1), active: map[int64]*activeBuild{}}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// Start marks builds a previous process left running as errors, removes
// their containers and volumes, and starts the workers.
func (s *Service) Start(ctx context.Context) error {
	if err := s.store.Interrupt(ctx); err != nil {
		return err
	}
	if c, ok := s.rt.(Cleaner); ok {
		if err := c.RemoveLabeled(ctx, pipeline.Label); err != nil {
			s.log.Warn("ci: removing leftovers of earlier builds failed", "err", err)
		}
	}
	for range s.opts.Workers {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.work()
		}()
	}
	s.poke()
	return nil
}

// Close cancels running builds and waits for the workers.
func (s *Service) Close() {
	s.mu.Lock()
	for _, a := range s.active {
		a.cancel(errors.New("the server is stopping"))
	}
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
}

func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// work runs queued builds until the service closes.
func (s *Service) work() {
	tick := time.NewTicker(30 * time.Second) // in case a wake-up was missed
	defer tick.Stop()
	for {
		for s.ctx.Err() == nil {
			b, ok := s.claim()
			if !ok {
				break
			}
			s.run(b)
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-tick.C:
		}
	}
}

// claim marks the oldest queued build running and returns it.
func (s *Service) claim() (store.Build, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, err := s.store.Pending(s.ctx)
	if err != nil || len(pending) == 0 {
		return store.Build{}, false
	}
	b := pending[0]
	b.Status, b.StartedAt = pipeline.Running, time.Now()
	if err := s.store.UpdateBuild(s.ctx, b); err != nil {
		return store.Build{}, false
	}
	ctx, cancel := context.WithCancelCause(s.ctx)
	s.active[b.ID] = &activeBuild{ctx: ctx, cancel: cancel, logs: map[int]*logBuffer{}}
	return b, true
}

// run runs a claimed build to the end.
func (s *Service) run(b store.Build) {
	s.mu.Lock()
	a := s.active[b.ID]
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.active, b.ID)
		s.mu.Unlock()
		s.poke() // another worker may be waiting for this one's slot
	}()
	repo, err := s.store.Repo(s.ctx, b.RepoID)
	if err != nil {
		b.Status, b.Error, b.FinishedAt = pipeline.Error, err.Error(), time.Now()
		s.store.UpdateBuild(context.WithoutCancel(s.ctx), b)
		return
	}
	defer a.cancel(nil)
	timeout := time.Duration(repo.Timeout) * time.Minute
	runCtx, stop := context.WithTimeoutCause(a.ctx, timeout, fmt.Errorf("timed out after %d minutes", repo.Timeout))
	defer stop()

	rep := &reporter{s: s, build: &b, active: a}
	spec := pipeline.Spec{
		BuildID: b.ID, Number: b.Number, RepoID: repo.ID, RepoName: repo.Name, URL: repo.URL, Token: repo.Token,
		Branch: b.Branch, Commit: b.Commit, Event: b.Event, ConfigPath: repo.ConfigPath, Secrets: repo.Secrets,
	}
	status, reason := pipeline.Run(runCtx, s.rt, spec, s.opts.Pipeline, rep)
	if status == pipeline.Canceled {
		if cause := context.Cause(runCtx); cause != nil && !errors.Is(cause, errCanceled) && !errors.Is(cause, context.Canceled) {
			reason = cause.Error()
		}
	}
	bg := context.WithoutCancel(s.ctx)
	// Steps that never ran end as skipped (or canceled with the build).
	steps, _ := s.store.Steps(bg, b.ID)
	for _, st := range steps {
		if st.Status == pipeline.Pending || st.Status == pipeline.Running {
			st.Status = pipeline.Skipped
			if status == pipeline.Canceled {
				st.Status = pipeline.Canceled
			}
			s.store.UpdateStep(bg, st, nil)
		}
	}
	b.Status, b.Error, b.FinishedAt = status, reason, time.Now()
	if err := s.store.UpdateBuild(bg, b); err != nil {
		s.log.Error("ci: recording the build failed", "build", b.ID, "err", err)
	}
	s.store.Prune(bg, repo.ID, s.opts.Keep)
	s.log.Info("ci: build finished", "repo", repo.Name, "number", b.Number, "status", status, "error", reason)
}

// reporter records a build's progress in the store and the live logs.
type reporter struct {
	s      *Service
	build  *store.Build
	active *activeBuild
	steps  []store.Step
}

func (r *reporter) bg() context.Context { return context.WithoutCancel(r.s.ctx) }

func (r *reporter) Steps(names []string) {
	r.s.store.SetSteps(r.bg(), r.build.ID, names)
	for i := len(r.steps); i < len(names); i++ {
		r.steps = append(r.steps, store.Step{BuildID: r.build.ID, Position: i, Name: names[i], Status: pipeline.Pending})
	}
}

func (r *reporter) Commit(c pipeline.Commit) {
	r.build.Commit, r.build.Author, r.build.Message = c.SHA, c.Author, c.Message
	r.s.store.UpdateBuild(r.bg(), *r.build)
}

func (r *reporter) Start(i int) io.Writer {
	buf := &logBuffer{max: r.s.opts.MaxLog}
	r.s.mu.Lock()
	r.active.logs[i] = buf
	r.s.mu.Unlock()
	st := &r.steps[i]
	st.Status, st.StartedAt = pipeline.Running, time.Now()
	r.s.store.UpdateStep(r.bg(), *st, nil)
	return buf
}

func (r *reporter) Finish(i int, status string, code int) {
	st := &r.steps[i]
	st.Status, st.ExitCode, st.FinishedAt = status, code, time.Now()
	var log *string
	r.s.mu.Lock()
	if buf := r.active.logs[i]; buf != nil {
		text := buf.String()
		log = &text
	}
	r.s.mu.Unlock()
	r.s.store.UpdateStep(r.bg(), *st, log)
}

// ---- repositories ----

// RepoInput creates or updates a repository. On update an empty token
// keeps the stored one; secrets map names to values, an empty value
// keeping the stored one, and names left out are removed.
type RepoInput struct {
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Branch     string            `json:"branch"`
	ConfigPath string            `json:"config_path"`
	Token      string            `json:"token"`
	Timeout    int               `json:"timeout"`
	Secrets    map[string]string `json:"secrets"`
}

func (in RepoInput) apply(r *store.Repo) error {
	r.Name, r.URL = strings.TrimSpace(in.Name), strings.TrimSpace(in.URL)
	r.Branch, r.ConfigPath, r.Timeout = strings.TrimSpace(in.Branch), strings.TrimSpace(in.ConfigPath), in.Timeout
	if r.Branch == "" {
		r.Branch = "main"
	}
	if r.ConfigPath == "" {
		r.ConfigPath = ".ci.yml"
	}
	if r.Timeout == 0 {
		r.Timeout = 60
	}
	if in.Token != "" {
		r.Token = in.Token
	}
	switch u, err := url.Parse(r.URL); {
	case r.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalid)
	case err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "git") || u.Host == "":
		return fmt.Errorf("%w: url must be an http(s), ssh or git URL", ErrInvalid)
	case r.Timeout < 1 || r.Timeout > 24*60:
		return fmt.Errorf("%w: timeout must be 1 to 1440 minutes", ErrInvalid)
	case strings.HasPrefix(r.ConfigPath, "/") || strings.Contains(r.ConfigPath, ".."):
		return fmt.Errorf("%w: config path must be inside the repository", ErrInvalid)
	}
	secrets := map[string]string{}
	for name, value := range in.Secrets {
		name = strings.TrimSpace(name)
		if !validName(name) {
			return fmt.Errorf("%w: secret name %q must be letters, digits and _", ErrInvalid, name)
		}
		if value == "" {
			value = r.Secrets[name]
		}
		if value == "" {
			return fmt.Errorf("%w: secret %s has no value", ErrInvalid, name)
		}
		secrets[name] = value
	}
	r.Secrets = secrets
	return nil
}

func validName(s string) bool {
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		return false
	}
	for _, c := range s {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func randomSecret() string {
	b := make([]byte, 20)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func view(r store.Repo, last *store.Build, withSecret bool) RepoView {
	v := RepoView{Repo: r, HasToken: r.Token != "", SecretNames: []string{}, LastBuild: last}
	for name := range r.Secrets {
		v.SecretNames = append(v.SecretNames, name)
	}
	sort.Strings(v.SecretNames)
	if withSecret {
		v.WebhookSecret = r.WebhookSecret
	}
	return v
}

// Repos returns every repository with its latest build.
func (s *Service) Repos(ctx context.Context) ([]RepoView, error) {
	repos, err := s.store.Repos(ctx)
	if err != nil {
		return nil, err
	}
	last, err := s.store.LastBuilds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RepoView, len(repos))
	for i, r := range repos {
		var lb *store.Build
		if b, ok := last[r.ID]; ok {
			lb = &b
		}
		out[i] = view(r, lb, false)
	}
	return out, nil
}

// Repo returns one repository, with its webhook secret.
func (s *Service) Repo(ctx context.Context, id int64) (RepoView, error) {
	r, err := s.store.Repo(ctx, id)
	if err != nil {
		return RepoView{}, err
	}
	builds, _ := s.store.Builds(ctx, id, 1)
	var lb *store.Build
	if len(builds) > 0 {
		lb = &builds[0]
	}
	return view(r, lb, true), nil
}

// CreateRepo adds a repository with a fresh webhook secret.
func (s *Service) CreateRepo(ctx context.Context, in RepoInput) (RepoView, error) {
	r := store.Repo{WebhookSecret: randomSecret()}
	if err := in.apply(&r); err != nil {
		return RepoView{}, err
	}
	if err := s.store.CreateRepo(ctx, &r); err != nil {
		return RepoView{}, err
	}
	return s.Repo(ctx, r.ID)
}

// UpdateRepo changes a repository.
func (s *Service) UpdateRepo(ctx context.Context, id int64, in RepoInput) (RepoView, error) {
	r, err := s.store.Repo(ctx, id)
	if err != nil {
		return RepoView{}, err
	}
	if err := in.apply(&r); err != nil {
		return RepoView{}, err
	}
	if err := s.store.UpdateRepo(ctx, &r); err != nil {
		return RepoView{}, err
	}
	return s.Repo(ctx, id)
}

// RotateWebhookSecret replaces a repository's webhook secret.
func (s *Service) RotateWebhookSecret(ctx context.Context, id int64) (RepoView, error) {
	r, err := s.store.Repo(ctx, id)
	if err != nil {
		return RepoView{}, err
	}
	r.WebhookSecret = randomSecret()
	if err := s.store.UpdateRepo(ctx, &r); err != nil {
		return RepoView{}, err
	}
	return s.Repo(ctx, id)
}

// DeleteRepo cancels a repository's running builds and removes it with
// its history.
func (s *Service) DeleteRepo(ctx context.Context, id int64) error {
	builds, err := s.store.Builds(ctx, id, 1000)
	if err != nil {
		return err
	}
	s.mu.Lock()
	for _, b := range builds {
		if a := s.active[b.ID]; a != nil {
			a.cancel(errCanceled)
		}
	}
	s.mu.Unlock()
	return s.store.DeleteRepo(ctx, id)
}

// ---- builds ----

// Trigger queues a build of a repository.
func (s *Service) Trigger(ctx context.Context, repoID int64, event, branch, commit, message, author string) (store.Build, error) {
	r, err := s.store.Repo(ctx, repoID)
	if err != nil {
		return store.Build{}, err
	}
	if branch == "" {
		branch = r.Branch
	}
	b := store.Build{RepoID: repoID, Event: event, Branch: branch, Commit: commit, Message: message, Author: author, Status: pipeline.Pending}
	if err := s.store.CreateBuild(ctx, &b); err != nil {
		return store.Build{}, err
	}
	s.poke()
	return b, nil
}

// Restart queues a new build of the same commit as build id.
func (s *Service) Restart(ctx context.Context, id int64) (store.Build, error) {
	b, err := s.store.Build(ctx, id)
	if err != nil {
		return store.Build{}, err
	}
	return s.Trigger(ctx, b.RepoID, b.Event, b.Branch, b.Commit, b.Message, b.Author)
}

// Cancel stops a running build or dequeues a pending one.
func (s *Service) Cancel(ctx context.Context, id int64) error {
	s.mu.Lock()
	a := s.active[id]
	s.mu.Unlock()
	if a != nil {
		a.cancel(errCanceled)
		return nil
	}
	b, err := s.store.Build(ctx, id)
	if err != nil {
		return err
	}
	if b.Status != pipeline.Pending {
		return fmt.Errorf("%w: the build has finished", ErrInvalid)
	}
	b.Status, b.FinishedAt = pipeline.Canceled, time.Now()
	return s.store.UpdateBuild(ctx, b)
}

// Build returns a build with its steps.
func (s *Service) Build(ctx context.Context, id int64) (store.Build, error) {
	return s.store.Build(ctx, id)
}

// Builds returns a repository's latest builds (all repositories' for 0).
func (s *Service) Builds(ctx context.Context, repoID int64) ([]store.Build, error) {
	if repoID > 0 {
		if _, err := s.store.Repo(ctx, repoID); err != nil {
			return nil, err
		}
	}
	return s.store.Builds(ctx, repoID, s.opts.Keep)
}

// Log is part of a step's log.
type Log struct {
	Text   string `json:"text"`
	Offset int64  `json:"offset"` // pass back to get what follows
	Done   bool   `json:"done"`   // the step has finished
}

// StepLog returns a step's log from offset: live while the step runs,
// stored once it has finished.
func (s *Service) StepLog(ctx context.Context, buildID int64, position int, offset int64) (Log, error) {
	s.mu.Lock()
	var buf *logBuffer
	if a := s.active[buildID]; a != nil {
		buf = a.logs[position]
	}
	s.mu.Unlock()
	steps, err := s.store.Steps(ctx, buildID)
	if err != nil {
		return Log{}, err
	}
	if position < 0 || position >= len(steps) {
		return Log{}, store.ErrNotFound
	}
	st := steps[position]
	if buf != nil && st.Status == pipeline.Running {
		text, next := buf.from(offset)
		return Log{Text: text, Offset: next}, nil
	}
	text, err := s.store.StepLog(ctx, buildID, position)
	if err != nil {
		return Log{}, err
	}
	done := st.Status != pipeline.Pending && st.Status != pipeline.Running
	if offset >= int64(len(text)) {
		return Log{Offset: int64(len(text)), Done: done}, nil
	}
	return Log{Text: text[offset:], Offset: int64(len(text)), Done: done}, nil
}

// Summary counts repositories and builds.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	repos, err := s.Repos(ctx)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Repos: len(repos)}
	for _, r := range repos {
		if r.LastBuild == nil {
			continue
		}
		switch r.LastBuild.Status {
		case pipeline.Running:
			sum.Running++
		case pipeline.Pending:
			sum.Pending++
		case pipeline.Success, pipeline.Skipped, pipeline.Canceled:
		default:
			sum.Failing++
		}
	}
	return sum, nil
}

// logBuffer keeps the end of a step's log and where it starts in the
// whole log, so readers can follow it by offset.
type logBuffer struct {
	mu   sync.Mutex
	max  int
	base int64 // offset of data[0] in the whole log
	data []byte
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data = append(l.data, p...)
	if over := len(l.data) - l.max; over > 0 {
		l.data = append(l.data[:0], l.data[over:]...)
		l.base += int64(over)
	}
	return len(p), nil
}

// from returns what follows offset, and the offset after it.
func (l *logBuffer) from(offset int64) (string, int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	end := l.base + int64(len(l.data))
	if offset >= end {
		return "", end
	}
	if offset < l.base {
		return "…\n" + string(l.data), end
	}
	return string(l.data[offset-l.base:]), end
}

// String returns the kept log, marked when its start was dropped.
func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.base > 0 {
		return "…(earlier output omitted)\n" + string(l.data)
	}
	return string(l.data)
}
