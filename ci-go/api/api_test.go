package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

	"github.com/lsongdev/ci-go/api"
	"github.com/lsongdev/ci-go/docker"
	"github.com/lsongdev/ci-go/pipeline"
	"github.com/lsongdev/ci-go/store"
)

// fakeRuntime pretends to run containers: the clone succeeds, the prepare
// container prints the commit and config, and steps print their script;
// a script with "sleep" runs until cancelled and one with "false" fails.
type fakeRuntime struct {
	mu      sync.Mutex
	config  string
	volumes map[string]bool
}

func (f *fakeRuntime) ImageExists(context.Context, string) (bool, error) { return true, nil }
func (f *fakeRuntime) Pull(context.Context, string, io.Writer) error     { return nil }
func (f *fakeRuntime) CreateVolume(_ context.Context, name string, _ map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.volumes[name] = true
	return nil
}
func (f *fakeRuntime) RemoveVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.volumes, name)
	return nil
}

func (f *fakeRuntime) Run(ctx context.Context, spec docker.Container, out io.Writer) (int, error) {
	env := strings.Join(spec.Env, "\n")
	script := strings.Join(spec.Cmd, " ")
	switch {
	case strings.Contains(env, "CI_REPO_URL="):
		fmt.Fprintln(out, "cloned")
		return 0, nil
	case strings.Contains(env, "CI_CONFIG="):
		f.mu.Lock()
		defer f.mu.Unlock()
		fmt.Fprintf(out, "abc123\x1fAlice\x1fa change\n--- ci-go config ---\n%s", f.config)
		return 0, nil
	case strings.Contains(script, "sleep"):
		fmt.Fprintln(out, "sleeping")
		<-ctx.Done()
		return -1, ctx.Err()
	case strings.Contains(script, "false"):
		return 1, nil
	}
	fmt.Fprint(out, script)
	return 0, nil
}

type env struct {
	t   *testing.T
	rt  *fakeRuntime
	svc *api.Service
	h   http.Handler
}

func newEnv(t *testing.T, config string) *env {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	rt := &fakeRuntime{config: config, volumes: map[string]bool{}}
	svc := api.NewService(st, rt, api.Options{})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return &env{t: t, rt: rt, svc: svc, h: svc.Handler()}
}

func (e *env) call(method, path string, body any, want int, out any, headers ...string) {
	e.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		r = bytes.NewReader(b)
	default:
		data, _ := json.Marshal(b)
		r = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, r)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
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

// wait polls a build until it has finished.
func (e *env) wait(id int64) store.Build {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var b store.Build
		e.call("GET", fmt.Sprintf("/builds/%d", id), nil, 200, &b)
		if b.Status != pipeline.Pending && b.Status != pipeline.Running {
			return b
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("build %d still %s", id, b.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const config = `
steps:
  test: {image: alpine, commands: [echo testing]}
  fail: {image: alpine, commands: ["false"]}
  never: {image: alpine, commands: [echo never]}
`

func TestReposAndBuilds(t *testing.T) {
	e := newEnv(t, config)
	var repo api.RepoView
	e.call("POST", "/repos", map[string]any{"name": "app", "url": "https://git.example.com/app.git", "token": "t0ken", "secrets": map[string]string{"API_KEY": "k"}}, 201, &repo)
	if repo.Branch != "main" || repo.ConfigPath != ".ci.yml" || repo.Timeout != 60 || !repo.HasToken || repo.WebhookSecret == "" || repo.SecretNames[0] != "API_KEY" {
		t.Fatalf("repo %+v", repo)
	}
	e.call("POST", "/repos", map[string]any{"name": "app", "url": "https://x/y"}, 409, nil)
	for _, bad := range []map[string]any{{"url": "https://x/y"}, {"name": "b", "url": "not a url"}, {"name": "b", "url": "https://x/y", "secrets": map[string]string{"1X": "v"}},
		{"name": "b", "url": "https://x/y", "config_path": "../x"}} {
		e.call("POST", "/repos", bad, 422, nil)
	}
	var list []api.RepoView
	e.call("GET", "/repos", nil, 200, &list)
	if len(list) != 1 || list[0].WebhookSecret != "" {
		t.Fatalf("list must not show the webhook secret: %+v", list)
	}
	if body := fmt.Sprint(list); strings.Contains(body, "t0ken") {
		t.Fatal("token leaked")
	}

	var b store.Build
	e.call("POST", "/repos/1/builds", nil, 201, &b)
	if b.Number != 1 || b.Event != "manual" || b.Branch != "main" {
		t.Fatalf("queued %+v", b)
	}
	b = e.wait(b.ID)
	if b.Status != pipeline.Failure || b.Error != "fail exited with code 1" || b.Commit != "abc123" || b.Author != "Alice" {
		t.Fatalf("build %+v", b)
	}
	got := []string{}
	for _, st := range b.Steps {
		got = append(got, st.Name+":"+st.Status)
	}
	if strings.Join(got, " ") != "clone:success test:success fail:failure never:skipped" {
		t.Fatalf("steps %v", got)
	}
	var log api.Log
	e.call("GET", fmt.Sprintf("/builds/%d/steps/1/log", b.ID), nil, 200, &log)
	if !log.Done || !strings.Contains(log.Text, "echo testing") {
		t.Fatalf("log %+v", log)
	}
	var more api.Log
	e.call("GET", fmt.Sprintf("/builds/%d/steps/1/log?offset=%d", b.ID, log.Offset), nil, 200, &more)
	if more.Text != "" || more.Offset != log.Offset {
		t.Fatalf("log after the end %+v", more)
	}
	if len(e.rt.volumes) != 0 {
		t.Fatalf("workspace left: %v", e.rt.volumes)
	}

	// Restarting builds the same commit again with the next number.
	var again store.Build
	e.call("POST", fmt.Sprintf("/builds/%d/restart", b.ID), nil, 201, &again)
	if again.Number != 2 || again.Commit != "abc123" {
		t.Fatalf("restart %+v", again)
	}
	e.wait(again.ID)
	var sum api.Summary
	e.call("GET", "/summary", nil, 200, &sum)
	if sum.Repos != 1 || sum.Failing != 1 {
		t.Fatalf("summary %+v", sum)
	}

	// Updating keeps the token and secrets left empty.
	e.call("PUT", "/repos/1", map[string]any{"name": "app", "url": "https://git.example.com/app.git", "branch": "dev", "secrets": map[string]string{"API_KEY": ""}}, 200, &repo)
	if !repo.HasToken || repo.Branch != "dev" || len(repo.SecretNames) != 1 {
		t.Fatalf("updated %+v", repo)
	}
	e.call("DELETE", "/repos/1", nil, 204, nil)
	e.call("GET", fmt.Sprintf("/builds/%d", b.ID), nil, 404, nil)
}

func TestCancel(t *testing.T) {
	e := newEnv(t, "steps: {wait: {image: alpine, commands: [sleep 600]}, after: {image: alpine, commands: [echo x]}}")
	e.call("POST", "/repos", map[string]any{"name": "app", "url": "https://git.example.com/app.git"}, 201, nil)
	var running, queued store.Build
	e.call("POST", "/repos/1/builds", nil, 201, &running)
	e.call("POST", "/repos/1/builds", nil, 201, &queued) // one worker: this one waits

	// Follow the running step's live log; it exists once the clone is done.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var log api.Log
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/builds/%d/steps/1/log", running.ID), nil))
		var envelope struct{ Data api.Log }
		json.Unmarshal(w.Body.Bytes(), &envelope)
		log = envelope.Data
		if w.Code == 200 && strings.Contains(log.Text, "sleeping") && !log.Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live log %+v", log)
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.call("POST", fmt.Sprintf("/builds/%d/cancel", queued.ID), nil, 202, nil)
	if b := e.wait(queued.ID); b.Status != pipeline.Canceled {
		t.Fatalf("dequeued %+v", b)
	}
	e.call("POST", fmt.Sprintf("/builds/%d/cancel", running.ID), nil, 202, nil)
	b := e.wait(running.ID)
	if b.Status != pipeline.Canceled || b.Steps[1].Status != pipeline.Canceled || b.Steps[2].Status != pipeline.Skipped {
		t.Fatalf("canceled %+v", b)
	}
	e.call("POST", fmt.Sprintf("/builds/%d/cancel", running.ID), nil, 422, nil)
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhooks(t *testing.T) {
	e := newEnv(t, "steps: {test: {image: alpine, commands: [echo ok]}}")
	var repo api.RepoView
	e.call("POST", "/repos", map[string]any{"name": "app", "url": "https://git.example.com/app.git"}, 201, &repo)
	secret := repo.WebhookSecret
	github := []byte(`{"ref":"refs/heads/feature","after":"0123456789abcdef","head_commit":{"message":"add thing\n\nlonger text","author":{"name":"Bob"}}}`)

	e.call("POST", "/hooks/1", github, 401, nil, "X-GitHub-Event", "push", "X-Hub-Signature-256", "sha256="+sign("wrong", github))
	e.call("POST", "/hooks/1", github, 401, nil, "X-GitHub-Event", "push")
	e.call("POST", "/hooks/99", github, 404, nil, "X-GitHub-Event", "push", "X-Hub-Signature-256", "sha256="+sign(secret, github))
	e.call("POST", "/hooks/1", []byte(`{}`), 200, nil, "X-GitHub-Event", "ping", "X-Hub-Signature-256", "sha256="+sign(secret, []byte(`{}`)))

	var b store.Build
	e.call("POST", "/hooks/1", github, 201, &b, "X-GitHub-Event", "push", "X-Hub-Signature-256", "sha256="+sign(secret, github))
	if b.Event != "push" || b.Branch != "feature" || b.Commit != "0123456789abcdef" || b.Message != "add thing" || b.Author != "Bob" {
		t.Fatalf("github build %+v", b)
	}
	e.wait(b.ID)

	// Gitea signs the same way under its own header.
	e.call("POST", "/hooks/1", github, 201, nil, "X-Gitea-Event", "push", "X-Gitea-Signature", sign(secret, github))
	// GitLab sends the secret as a token.
	gitlab := []byte(`{"ref":"refs/heads/main","checkout_sha":"feedface","user_name":"Carol","commits":[{"id":"feedface","message":"fix"}]}`)
	e.call("POST", "/hooks/1", gitlab, 201, &b, "X-Gitlab-Event", "Push Hook", "X-Gitlab-Token", secret)
	if b.Commit != "feedface" || b.Author != "Carol" || b.Message != "fix" {
		t.Fatalf("gitlab build %+v", b)
	}
	e.call("POST", "/hooks/1", gitlab, 401, nil, "X-Gitlab-Event", "Push Hook", "X-Gitlab-Token", "nope")

	// Tags, deleted branches and other events start nothing.
	for _, body := range []string{
		`{"ref":"refs/tags/v1","after":"abc"}`,
		`{"ref":"refs/heads/gone","after":"0000000000000000000000000000000000000000"}`,
	} {
		e.call("POST", "/hooks/1", []byte(body), 202, nil, "X-GitHub-Event", "push", "X-Hub-Signature-256", "sha256="+sign(secret, []byte(body)))
	}
	e.call("POST", "/hooks/1", []byte(`{}`), 202, nil, "X-GitHub-Event", "issues", "X-Hub-Signature-256", "sha256="+sign(secret, []byte(`{}`)))

	// A rotated secret invalidates the old one.
	e.call("POST", "/repos/1/webhook-secret", nil, 200, &repo)
	e.call("POST", "/hooks/1", github, 401, nil, "X-GitHub-Event", "push", "X-Hub-Signature-256", "sha256="+sign(secret, github))
}
