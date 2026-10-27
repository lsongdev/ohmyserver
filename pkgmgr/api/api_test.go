package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lsongdev/ohmyserver/pkgmgr/system"
)

// fakeManager installs instantly, or blocks installs until release closes.
type fakeManager struct {
	release chan struct{}
}

func (*fakeManager) Name() string { return "fake" }
func (*fakeManager) Installed(context.Context) ([]system.Package, error) {
	return []system.Package{{Name: "bash", Version: "5.1", Arch: "x86_64", Installed: true}}, nil
}
func (*fakeManager) Updates(context.Context) ([]system.Update, error) {
	return []system.Update{{Name: "bash", Arch: "x86_64", Version: "5.2"}}, nil
}
func (*fakeManager) Search(_ context.Context, q string) ([]system.Package, error) {
	return []system.Package{{Name: "bash"}, {Name: "bash-completion"}}, nil
}
func (*fakeManager) Info(context.Context, string) (string, error) { return "Name : bash", nil }
func (*fakeManager) Refresh(context.Context, func(string)) error  { return nil }
func (m *fakeManager) Install(_ context.Context, names []string, out func(string)) error {
	out("Installing " + strings.Join(names, " "))
	if m.release != nil {
		<-m.release
	}
	return nil
}
func (*fakeManager) Remove(_ context.Context, names []string, out func(string)) error {
	return &system.ExitError{Command: "dnf remove", Code: 1, Output: "Error: No match for argument: " + names[0]}
}
func (*fakeManager) Upgrade(context.Context, []string, func(string)) error { return nil }

func call(t *testing.T, h http.Handler, method, path, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestJobs(t *testing.T) {
	m := &fakeManager{release: make(chan struct{})}
	s := NewService(Options{Manager: m, Root: t.TempDir()})
	h := s.Handler()

	status, out := call(t, h, "POST", "/jobs", `{"action":"install","packages":["tree"]}`)
	if status != http.StatusAccepted {
		t.Fatalf("install: %d %s", status, out["error"])
	}
	// One change at a time.
	if status, out = call(t, h, "POST", "/jobs", `{"action":"upgrade"}`); status != http.StatusConflict {
		t.Fatalf("second job: %d %s", status, out["error"])
	}
	if status, out = call(t, h, "GET", "/system", ""); !strings.Contains(string(out["data"]), `"title":"Install tree"`) {
		t.Fatalf("system: %s", out["data"])
	}
	close(m.release)
	s.Wait()

	_, out = call(t, h, "GET", "/jobs/1", "")
	var job Job
	json.Unmarshal(out["data"], &job)
	if job.Status != "succeeded" || len(job.Output) != 1 || job.Output[0] != "Installing tree" || job.FinishedAt == nil {
		t.Fatalf("job %+v", job)
	}

	call(t, h, "POST", "/jobs", `{"action":"remove","packages":["nope"]}`)
	s.Wait()
	_, out = call(t, h, "GET", "/jobs", "")
	var jobs []Job
	json.Unmarshal(out["data"], &jobs)
	if len(jobs) != 2 || jobs[0].Status != "failed" || !strings.Contains(jobs[0].Error, "No match") || jobs[0].Output != nil {
		t.Fatalf("jobs %+v", jobs)
	}
}

func TestRejectsBadRequests(t *testing.T) {
	h := NewService(Options{Manager: &fakeManager{}, Root: t.TempDir()}).Handler()
	for _, body := range []string{
		`{"action":"install","packages":["--setopt=x"]}`,
		`{"action":"install"}`,
		`{"action":"reboot"}`,
		`{"action":"install","packages":["a"],"extra":1}`,
	} {
		if status, _ := call(t, h, "POST", "/jobs", body); status != http.StatusBadRequest {
			t.Errorf("%s: status %d", body, status)
		}
	}
}

func TestUpdatesShowCurrentVersion(t *testing.T) {
	h := NewService(Options{Manager: &fakeManager{}, Root: t.TempDir()}).Handler()
	_, out := call(t, h, "GET", "/updates", "")
	if !strings.Contains(string(out["data"]), `"version":"5.2","current":"5.1"`) {
		t.Fatalf("updates %s", out["data"])
	}
	_, out = call(t, h, "GET", "/search?q=bash", "")
	if !strings.Contains(string(out["data"]), `{"name":"bash","version":"","installed":true}`) {
		t.Fatalf("search %s", out["data"])
	}
}

func TestUnsupported(t *testing.T) {
	s := &Service{os: system.OS{ID: "darwin"}} // no package manager
	h := s.Handler()
	if status, out := call(t, h, "GET", "/installed", ""); status != http.StatusServiceUnavailable || !strings.Contains(string(out["error"]), "unsupported") {
		t.Fatalf("installed: %d %s", status, out["error"])
	}
	if status, _ := call(t, h, "GET", "/system", ""); status != http.StatusOK {
		t.Fatalf("system: %d", status)
	}
}
