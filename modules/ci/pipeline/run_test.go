package pipeline

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/ci/docker"
)

// recorder is a Reporter that keeps everything.
type recorder struct {
	mu     sync.Mutex
	names  []string
	commit Commit
	logs   map[int]*bytes.Buffer
	status map[int]string
	codes  map[int]int
}

func newRecorder() *recorder {
	return &recorder{logs: map[int]*bytes.Buffer{}, status: map[int]string{}, codes: map[int]int{}}
}

func (r *recorder) Steps(names []string) { r.mu.Lock(); r.names = names; r.mu.Unlock() }
func (r *recorder) Commit(c Commit)      { r.mu.Lock(); r.commit = c; r.mu.Unlock() }
func (r *recorder) Start(i int) io.Writer {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs[i] = &bytes.Buffer{}
	return lockedWriter{&r.mu, r.logs[i]}
}
func (r *recorder) Finish(i int, status string, code int) {
	r.mu.Lock()
	r.status[i], r.codes[i] = status, code
	r.mu.Unlock()
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (r *recorder) log(i int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b := r.logs[i]; b != nil {
		return strings.ReplaceAll(b.String(), "\r\n", "\n")
	}
	return ""
}

func dockerClient(t *testing.T) *docker.Client {
	t.Helper()
	c, err := docker.FromEnv()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Skip("no Docker daemon:", err)
	}
	return c
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_COMMITTER_NAME=Alice", "GIT_COMMITTER_EMAIL=a@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitServer serves the repositories under root over smart HTTP and
// returns their base URL as containers reach it.
func gitServer(t *testing.T, root string) string {
	t.Helper()
	execPath, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Skip("git not installed")
	}
	backend := &cgi.Handler{
		Path: filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend"),
		Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: backend}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return "http://host.docker.internal:" + port
}

// repo creates a repository with files on branch main and returns its URL.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	os.MkdirAll(work, 0o755)
	git(t, work, "init", "-q", "-b", "main")
	for name, content := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(work, name)), 0o755)
		os.WriteFile(filepath.Join(work, name), []byte(content), 0o644)
	}
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "first commit")
	git(t, root, "clone", "-q", "--bare", "work", "app.git")
	return gitServer(t, root) + "/app.git"
}

func TestRun(t *testing.T) {
	rt := dockerClient(t)
	url := repo(t, map[string]string{
		"hello.txt": "hello from the repo\n",
		".ci.yml": `
when: {branch: [main], event: [push, manual]}
steps:
  greet:
    image: alpine:3
    commands:
      - cat hello.txt
      - echo "branch=$CI_COMMIT_BRANCH number=$CI_BUILD_NUMBER mode=$MODE secret=$TOKEN"
      - echo cached >> /cache/runs
      - wc -l < /cache/runs
    environment: {MODE: test}
    secrets: [TOKEN]
    cache: [/cache]
  fail:
    image: alpine:3
    commands: [echo failing, "false"]
  after:
    image: alpine:3
    commands: [echo never]
`,
	})
	spec := Spec{BuildID: 900001, Number: 7, RepoID: 900001, RepoName: "app", URL: url, Branch: "main", Event: "manual",
		Secrets: map[string]string{"TOKEN": "s3cr3t-value"}}
	defer rt.RemoveVolume(context.Background(), cacheVolume(spec.RepoID, "/cache"))
	rec := newRecorder()
	status, reason := Run(context.Background(), rt, spec, Options{}, rec)
	if status != Failure || reason != "fail exited with code 1" {
		t.Fatalf("status %s %q; clone log:\n%s", status, reason, rec.log(0))
	}
	if strings.Join(rec.names, ",") != "clone,greet,fail,after" {
		t.Fatalf("steps %v", rec.names)
	}
	if rec.commit.Author != "Alice" || rec.commit.Message != "first commit" || len(rec.commit.SHA) != 40 {
		t.Fatalf("commit %+v", rec.commit)
	}
	greet := rec.log(1)
	for _, want := range []string{"+ cat hello.txt", "hello from the repo", "branch=main number=7 mode=test secret=********", "\n1\n"} {
		if !strings.Contains(greet, want) {
			t.Errorf("greet log lacks %q:\n%s", want, greet)
		}
	}
	if strings.Contains(greet, "s3cr3t-value") {
		t.Error("secret leaked into the log")
	}
	if rec.status[0] != Success || rec.status[1] != Success || rec.status[2] != Failure || rec.codes[2] != 1 || rec.status[3] != Skipped {
		t.Fatalf("statuses %v codes %v", rec.status, rec.codes)
	}

	// The cache volume outlives the build; the workspace does not.
	rec = newRecorder()
	spec.BuildID++
	Run(context.Background(), rt, spec, Options{}, rec)
	if !strings.Contains(rec.log(1), "\n2\n") {
		t.Errorf("cache not kept:\n%s", rec.log(1))
	}

	// Branches outside when are skipped after reading the configuration.
	rec = newRecorder()
	spec.BuildID++
	spec.Event = "pull_request"
	if status, _ := Run(context.Background(), rt, spec, Options{}, rec); status != Skipped {
		t.Fatalf("filtered build: %s", status)
	}
}

func TestRunErrors(t *testing.T) {
	rt := dockerClient(t)
	url := repo(t, map[string]string{"README": "no config here\n"})
	rec := newRecorder()
	status, reason := Run(context.Background(), rt, Spec{BuildID: 900010, URL: url, Branch: "main", Event: "manual"}, Options{}, rec)
	if status != Error || !strings.Contains(reason, ".ci.yml not found") {
		t.Fatalf("no config: %s %q", status, reason)
	}
	rec = newRecorder()
	status, reason = Run(context.Background(), rt, Spec{BuildID: 900011, URL: url, Branch: "nope", Event: "manual"}, Options{}, rec)
	if status != Failure || !strings.Contains(reason, "clone exited") || rec.status[0] != Failure {
		t.Fatalf("missing branch: %s %q\n%s", status, reason, rec.log(0))
	}

	// Cancelling stops the running step.
	url = repo(t, map[string]string{".ci.yml": "steps: {wait: {image: alpine:3, commands: [sleep 60]}}"})
	ctx, cancel := context.WithCancel(context.Background())
	rec = newRecorder()
	go func() {
		for {
			rec.mu.Lock()
			started := rec.logs[1] != nil
			rec.mu.Unlock()
			if started {
				time.Sleep(500 * time.Millisecond)
				cancel()
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	start := time.Now()
	if status, reason := Run(ctx, rt, Spec{BuildID: 900012, URL: url, Branch: "main", Event: "manual"}, Options{}, rec); status != Canceled || time.Since(start) > 30*time.Second {
		t.Fatalf("cancel: %s %q after %v\n%s\n%s", status, reason, time.Since(start), rec.log(0), rec.log(1))
	}
}

// A build step builds an image with the daemon, from the workspace.
func TestBuildStep(t *testing.T) {
	if testing.Short() {
		t.Skip("pulls docker:cli")
	}
	rt := dockerClient(t)
	url := repo(t, map[string]string{
		"Dockerfile": "FROM alpine:3\nARG GREETING\nRUN echo \"$GREETING\" > /greeting\n",
		".ci.yml":    "steps:\n  image:\n    build: {tag: ci-go-test-image:latest, args: {GREETING: hi}}\n",
	})
	rec := newRecorder()
	status, reason := Run(context.Background(), rt, Spec{BuildID: 900020, URL: url, Branch: "main", Event: "manual"}, Options{}, rec)
	if status != Success {
		t.Fatalf("%s %q\n%s", status, reason, rec.log(1))
	}
	if ok, _ := rt.ImageExists(context.Background(), "ci-go-test-image:latest"); !ok {
		t.Fatalf("image not built:\n%s", rec.log(1))
	}
	if !strings.Contains(rec.log(1), "+ docker build --progress plain -t ci-go-test-image:latest --build-arg GREETING=hi .") {
		t.Errorf("log:\n%s", rec.log(1))
	}
	exec.Command("docker", "rmi", "ci-go-test-image:latest").Run()
}

func TestMasker(t *testing.T) {
	var out bytes.Buffer
	m := newMasker(&out, []string{"supersecret", "abc"})
	m.Write([]byte("token=super"))
	m.Write([]byte("secret here\nabc stays: short values are not masked\npartial supersec"))
	m.Flush()
	if got := out.String(); got != "token=******** here\nabc stays: short values are not masked\npartial supersec" {
		t.Fatalf("%q", got)
	}
}
