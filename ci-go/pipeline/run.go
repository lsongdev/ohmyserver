package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/lsongdev/ci-go/docker"
)

// Runtime runs containers; docker.Client implements it.
type Runtime interface {
	ImageExists(ctx context.Context, image string) (bool, error)
	Pull(ctx context.Context, image string, out io.Writer) error
	Run(ctx context.Context, spec docker.Container, out io.Writer) (int, error)
	CreateVolume(ctx context.Context, name string, labels map[string]string) error
	RemoveVolume(ctx context.Context, name string) error
}

// Statuses of builds and steps.
const (
	Pending  = "pending"
	Running  = "running"
	Success  = "success"
	Failure  = "failure"  // a step exited non-zero
	Error    = "error"    // the build could not run: clone, configuration, Docker
	Canceled = "canceled" // stopped by the user or a timeout
	Skipped  = "skipped"  // excluded by when, or after an earlier failure
)

// Label marks every container and volume a build creates.
const Label = "ci-go.build"

// Spec is one build to run.
type Spec struct {
	BuildID    int64
	Number     int
	RepoID     int64
	RepoName   string
	URL        string // git clone URL
	Token      string // for HTTPS clones of private repositories
	Branch     string
	Commit     string // a SHA; empty checks out Branch
	Event      string // push | manual
	ConfigPath string // .ci.yml by default
	Secrets    map[string]string
}

// Options are the build host's settings.
type Options struct {
	CloneImage   string // with git and sh; alpine/git by default
	BuilderImage string // with the docker CLI; docker:cli by default
	DaemonSocket string // the daemon's socket as the daemon sees it; /var/run/docker.sock
	Memory       int64  // per step container, bytes; 0 is unlimited
	NanoCPUs     int64  // per step container; 0 is unlimited
}

func (o *Options) defaults() {
	if o.CloneImage == "" {
		o.CloneImage = "alpine/git:latest"
	}
	if o.BuilderImage == "" {
		o.BuilderImage = "docker:cli"
	}
	if o.DaemonSocket == "" {
		o.DaemonSocket = "/var/run/docker.sock"
	}
}

// Commit is what a build checked out.
type Commit struct {
	SHA, Author, Message string
}

// Reporter receives a build's progress. Calls come from one goroutine.
type Reporter interface {
	// Steps announces the steps, clone first; it may be called again with
	// more steps once the configuration is read.
	Steps(names []string)
	// Commit reports the commit checked out.
	Commit(Commit)
	// Start reports that step i begins and returns where its log goes.
	Start(i int) io.Writer
	// Finish reports how step i ended.
	Finish(i int, status string, exitCode int)
}

// Run runs a build: it clones the repository into a fresh volume, reads
// the configuration, runs the steps in order and removes the volume. It
// returns the build's status and, for errors, why.
func Run(ctx context.Context, rt Runtime, spec Spec, opts Options, rep Reporter) (string, string) {
	opts.defaults()
	if spec.ConfigPath == "" {
		spec.ConfigPath = ".ci.yml"
	}
	labels := map[string]string{Label: strconv.FormatInt(spec.BuildID, 10)}
	workspace := fmt.Sprintf("ci-go-%d-workspace", spec.BuildID)
	if err := rt.CreateVolume(ctx, workspace, labels); err != nil {
		return Error, err.Error()
	}
	defer rt.RemoveVolume(context.WithoutCancel(ctx), workspace)
	ws := docker.Mount{Type: "volume", Source: workspace, Target: "/workspace"}
	env := spec.env()
	secretValues := make([]string, 0, len(spec.Secrets))
	for _, v := range spec.Secrets {
		secretValues = append(secretValues, v)
	}
	if spec.Token != "" {
		secretValues = append(secretValues, spec.Token, gitAuth(spec.Token))
	}

	rep.Steps([]string{"clone"})
	log := newMasker(rep.Start(0), secretValues)
	commit := spec.Commit
	if commit == "" {
		commit = spec.Branch
	}
	cloneEnv := append(env, "CI_REPO_URL="+spec.URL, "CI_CHECKOUT="+commit)
	if spec.Token != "" {
		cloneEnv = append(cloneEnv, "CI_GIT_AUTH="+gitAuth(spec.Token))
	}
	code, err := run(ctx, rt, docker.Container{
		Image: opts.CloneImage, Entrypoint: []string{"/bin/sh", "-c"}, Cmd: []string{cloneScript},
		Env: cloneEnv, WorkingDir: "/", Mounts: []docker.Mount{ws}, Labels: labels,
	}, false, log)
	log.Flush()
	if status := outcome(ctx, code, err); status != Success {
		rep.Finish(0, status, code)
		return status, failure(ctx, "clone", code, err)
	}
	rep.Finish(0, Success, 0)

	// Read the commit and the configuration without logging them.
	var out bytes.Buffer
	_, err = rt.Run(ctx, docker.Container{
		Image: opts.CloneImage, Entrypoint: []string{"/bin/sh", "-c"}, Cmd: []string{prepareScript},
		Env: append(env, "CI_CONFIG="+spec.ConfigPath), WorkingDir: "/workspace", Mounts: []docker.Mount{ws}, Labels: labels,
	}, &out)
	if err != nil {
		return outcome(ctx, -1, err), failure(ctx, "reading the configuration", -1, err)
	}
	info, config, found := strings.Cut(strings.ReplaceAll(out.String(), "\r\n", "\n"), "\n"+configMarker+"\n")
	if f := strings.SplitN(strings.TrimSpace(info), "\x1f", 3); len(f) == 3 {
		rep.Commit(Commit{SHA: f[0], Author: f[1], Message: f[2]})
	}
	if !found {
		return Error, spec.ConfigPath + " not found in the repository"
	}
	cfg, err := Parse([]byte(config))
	if err != nil {
		return Error, spec.ConfigPath + ": " + err.Error()
	}
	if !cfg.When.Match(spec.Event, spec.Branch) {
		return Skipped, ""
	}

	names := []string{"clone"}
	for _, s := range cfg.Steps {
		names = append(names, s.Name)
	}
	rep.Steps(names)
	status, reason := Success, ""
	for i, s := range cfg.Steps {
		n := i + 1
		if status != Success || !s.When.Match(spec.Event, spec.Branch) {
			rep.Finish(n, Skipped, 0)
			continue
		}
		log := newMasker(rep.Start(n), secretValues)
		code, err := runStep(ctx, rt, spec, opts, s, env, ws, labels, log)
		log.Flush()
		st := outcome(ctx, code, err)
		rep.Finish(n, st, code)
		if st != Success {
			status, reason = st, failure(ctx, s.Name, code, err)
		}
	}
	return status, reason
}

func runStep(ctx context.Context, rt Runtime, spec Spec, opts Options, s Step, env []string, ws docker.Mount, labels map[string]string, log io.Writer) (int, error) {
	mounts := []docker.Mount{ws}
	if s.Build != nil {
		args := []string{"build", "--progress", "plain"}
		for _, tag := range s.Build.Tag {
			args = append(args, "-t", tag)
		}
		if s.Build.Dockerfile != "" {
			args = append(args, "-f", s.Build.Dockerfile)
		}
		for _, k := range sortedKeys(s.Build.Args) {
			args = append(args, "--build-arg", k+"="+s.Build.Args[k])
		}
		args = append(args, s.Build.Context)
		fmt.Fprintf(log, "+ docker %s\n", strings.Join(args, " "))
		// The executor runs the build; steps never get the daemon.
		mounts = append(mounts, docker.Mount{Type: "bind", Source: opts.DaemonSocket, Target: "/var/run/docker.sock"})
		return run(ctx, rt, docker.Container{
			Image: opts.BuilderImage, Entrypoint: []string{"docker"}, Cmd: args, Env: []string{"DOCKER_BUILDKIT=1"},
			WorkingDir: "/workspace", Mounts: mounts, Labels: labels,
		}, false, log)
	}
	stepEnv := append([]string{}, env...)
	for _, k := range sortedKeys(s.Environment) {
		stepEnv = append(stepEnv, k+"="+s.Environment[k])
	}
	for _, name := range s.Secrets {
		v, ok := spec.Secrets[name]
		if !ok {
			return -1, fmt.Errorf("secret %q is not defined for this repository", name)
		}
		stepEnv = append(stepEnv, name+"="+v)
	}
	for _, p := range s.Cache {
		mounts = append(mounts, docker.Mount{Type: "volume", Source: cacheVolume(spec.RepoID, p), Target: p})
	}
	return run(ctx, rt, docker.Container{
		Image: s.Image, Entrypoint: []string{"/bin/sh", "-c"}, Cmd: []string{s.Script()}, Env: stepEnv,
		WorkingDir: "/workspace", Mounts: mounts, Labels: labels, Memory: opts.Memory, NanoCPUs: opts.NanoCPUs,
	}, s.Pull, log)
}

// run pulls the image when it is missing (or always, with pull) and runs
// the container.
func run(ctx context.Context, rt Runtime, spec docker.Container, pull bool, log io.Writer) (int, error) {
	present, err := rt.ImageExists(ctx, spec.Image)
	if err != nil {
		return -1, err
	}
	if pull || !present {
		fmt.Fprintf(log, "Pulling %s\n", spec.Image)
		if err := rt.Pull(ctx, spec.Image, log); err != nil {
			return -1, err
		}
	}
	return rt.Run(ctx, spec, log)
}

func outcome(ctx context.Context, code int, err error) string {
	switch {
	case ctx.Err() != nil:
		return Canceled
	case err != nil:
		return Error
	case code != 0:
		return Failure
	}
	return Success
}

func failure(ctx context.Context, step string, code int, err error) string {
	switch {
	case ctx.Err() != nil:
		return ""
	case err != nil:
		return step + ": " + err.Error()
	}
	return fmt.Sprintf("%s exited with code %d", step, code)
}

// env is what every step sees.
func (spec Spec) env() []string {
	return []string{
		"CI=true", "CI_WORKSPACE=/workspace", "GIT_PAGER=cat", // steps run with a TTY: keep git from paging
		"CI_REPO_NAME=" + spec.RepoName, "CI_BUILD_NUMBER=" + strconv.Itoa(spec.Number), "CI_BUILD_EVENT=" + spec.Event,
		"CI_COMMIT_BRANCH=" + spec.Branch, "CI_COMMIT_SHA=" + spec.Commit,
	}
}

// cloneScript clones CI_REPO_URL and checks out CI_CHECKOUT, a SHA or a
// branch. A token reaches git as a header, never in the URL or the log.
const cloneScript = `set -e
git config --global --add safe.directory /workspace
git config --global advice.detachedHead false
set --
if [ -n "$CI_GIT_AUTH" ]; then set -- -c "http.extraHeader=Authorization: Basic $CI_GIT_AUTH"; fi
echo "+ git clone $CI_REPO_URL"
git "$@" clone --quiet --no-checkout --filter=blob:none "$CI_REPO_URL" /workspace
cd /workspace
echo "+ git checkout $CI_CHECKOUT"
git "$@" checkout --quiet "$CI_CHECKOUT"
git log -1 --format='%h %s'`

const configMarker = "--- ci-go config ---"

// prepareScript prints the commit ("sha\x1fauthor\x1fsubject"), then the
// marker and the configuration if it exists.
const prepareScript = `git log -1 --format='%H%x1f%an%x1f%s'
if [ -f "$CI_CONFIG" ]; then echo '` + configMarker + `'; cat "$CI_CONFIG"; fi`

func gitAuth(token string) string {
	return base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
}

func cacheVolume(repoID int64, path string) string {
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("ci-go-cache-%d-%s", repoID, hex.EncodeToString(sum[:6]))
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// masker replaces secret values in a log with "********". It buffers up
// to a line, so a value split across writes is still caught.
type masker struct {
	mu      sync.Mutex
	out     io.Writer
	secrets []string
	buf     []byte
}

func newMasker(out io.Writer, secrets []string) *masker {
	var keep []string
	for _, s := range secrets {
		if len(s) >= 4 { // masking short values would garble the log
			keep = append(keep, s)
		}
	}
	return &masker{out: out, secrets: keep}
}

func (m *masker) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.secrets) == 0 {
		return m.out.Write(p)
	}
	m.buf = append(m.buf, p...)
	if i := bytes.LastIndexAny(m.buf, "\n\r"); i >= 0 {
		if err := m.emit(m.buf[:i+1]); err != nil {
			return 0, err
		}
		m.buf = append(m.buf[:0], m.buf[i+1:]...)
	}
	if len(m.buf) > 64<<10 { // a very long line: give up waiting for its end
		m.emit(m.buf)
		m.buf = m.buf[:0]
	}
	return len(p), nil
}

func (m *masker) emit(p []byte) error {
	s := string(p)
	for _, secret := range m.secrets {
		s = strings.ReplaceAll(s, secret, "********")
	}
	_, err := io.WriteString(m.out, s)
	return err
}

// Flush writes what is buffered.
func (m *masker) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.buf) > 0 {
		m.emit(m.buf)
		m.buf = m.buf[:0]
	}
}
