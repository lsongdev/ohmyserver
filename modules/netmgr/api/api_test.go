package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// fakeBackend records changes in memory.
type fakeBackend struct {
	mu                    sync.Mutex
	config                system.Config
	reason                string
	applied               int
	reverts               int
	setUps                []string
	failApply, failRevert error
}

func (f *fakeBackend) Name() string { return "systemd-networkd" }
func (f *fakeBackend) Read(context.Context, system.Link) (*system.Setting, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &system.Setting{Config: f.config, Version: "v1", Target: "file", Exists: true, Reason: f.reason}, nil
}
func (f *fakeBackend) Plan(_ context.Context, _ system.Link, c system.Config) (*system.Plan, error) {
	return &system.Plan{Target: "file", Before: "old\n", After: strings.Join(c.Addresses, "\n")}, nil
}
func (f *fakeBackend) Prepare(_ context.Context, l system.Link) (system.Undo, error) {
	return system.Undo{Backend: f.Name(), Link: l.Name}, nil
}
func (f *fakeBackend) Apply(_ context.Context, _ system.Link, c system.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failApply != nil {
		return f.failApply
	}
	f.applied++
	f.config = c
	return nil
}
func (f *fakeBackend) Revert(context.Context, system.Undo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRevert != nil {
		return f.failRevert
	}
	f.reverts++
	return nil
}
func (f *fakeBackend) SetUp(_ context.Context, name string, up bool) error {
	f.setUps = append(f.setUps, name)
	return nil
}
func (f *fakeBackend) reverted() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reverts
}

// ipRunner answers the iproute2 commands.
type ipRunner struct{ device string }

func (r ipRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, []byte, error) {
	call := name + " " + strings.Join(args, " ")
	switch {
	case strings.Contains(call, "addr show"):
		return []byte(`[
		 {"ifname":"eth0","flags":["UP","LOWER_UP"],"mtu":1500,"operstate":"UP","link_type":"ether","addr_info":[]},
		 {"ifname":"eth1","flags":["UP","LOWER_UP"],"mtu":1500,"operstate":"UP","link_type":"ether","addr_info":[]},
		 {"ifname":"docker0","flags":["UP"],"mtu":1500,"operstate":"DOWN","link_type":"ether","linkinfo":{"info_kind":"bridge"},"addr_info":[]}]`), nil, nil
	case strings.Contains(call, "-4 -j route"):
		return []byte(`[{"dst":"default","gateway":"10.0.0.1","dev":"eth0"}]`), nil, nil
	case strings.Contains(call, "route get"):
		return []byte(`[{"dev":"` + r.device + `"}]`), nil, nil
	}
	return []byte("[]"), nil, nil
}

func newService(t *testing.T, backend *fakeBackend, window time.Duration) (*Service, string) {
	t.Helper()
	yes := true
	dir := t.TempDir()
	s := New(Options{
		Runner: ipRunner{device: "lo"}, Backend: backend, Installed: &yes, StateDir: dir, Window: window,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

const goodBody = `{"version":"v1","config":{"addresses":["10.0.0.5/24"],"gateway4":"10.0.0.1"}}`

func TestApplyThenKeep(t *testing.T) {
	b := &fakeBackend{}
	s, dir := newService(t, b, time.Hour)
	h := s.Handler()
	code, out := call(t, h, "PUT", "/interfaces/eth0/config", goodBody)
	if code != 200 || b.applied != 1 {
		t.Fatalf("apply: %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, recordFile)); err != nil {
		t.Errorf("no record: %v", err)
	}
	if code, out := call(t, h, "GET", "/trial", ""); code != 200 || out["data"].(map[string]any)["trial"] == nil {
		t.Errorf("trial: %d %v", code, out)
	}
	// One at a time, and no switching interfaces off meanwhile.
	if code, _ := call(t, h, "PUT", "/interfaces/eth1/config", goodBody); code != 409 {
		t.Errorf("second change: %d", code)
	}
	if code, _ := call(t, h, "POST", "/interfaces/eth1/down", ""); code != 409 {
		t.Errorf("down during trial: %d", code)
	}
	if code, _ := call(t, h, "POST", "/trial/keep", ""); code != 200 {
		t.Errorf("keep: %d", code)
	}
	if b.reverted() != 0 {
		t.Error("reverted a kept change")
	}
	if _, err := os.Stat(filepath.Join(dir, recordFile)); !os.IsNotExist(err) {
		t.Errorf("record remains: %v", err)
	}
	if code, _ := call(t, h, "POST", "/trial/keep", ""); code != 409 {
		t.Errorf("keep twice: %d", code)
	}
}

func TestChangeRevertsByItself(t *testing.T) {
	b := &fakeBackend{}
	s, dir := newService(t, b, 20*time.Millisecond)
	if code, out := call(t, s.Handler(), "PUT", "/interfaces/eth0/config", goodBody); code != 200 {
		t.Fatalf("apply: %d %v", code, out)
	}
	deadline := time.Now().Add(2 * time.Second)
	for b.reverted() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if b.reverted() != 1 {
		t.Fatal("did not revert")
	}
	status := s.trial.status()
	if status.Trial != nil || status.Last == nil || status.Last.Result != "expired" {
		t.Errorf("status = %+v", status)
	}
	if _, err := os.Stat(filepath.Join(dir, recordFile)); !os.IsNotExist(err) {
		t.Errorf("record remains: %v", err)
	}
}

func TestRevertNow(t *testing.T) {
	b := &fakeBackend{}
	s, _ := newService(t, b, time.Hour)
	h := s.Handler()
	call(t, h, "PUT", "/interfaces/eth0/config", goodBody)
	if code, _ := call(t, h, "POST", "/trial/revert", ""); code != 200 || b.reverted() != 1 {
		t.Errorf("revert: %d, %d reverts", code, b.reverted())
	}
}

func TestFailedApplyRestoresAtOnce(t *testing.T) {
	b := &fakeBackend{failApply: &system.ExitError{Command: "networkctl", Code: 1, Output: "nope"}}
	s, dir := newService(t, b, time.Hour)
	code, out := call(t, s.Handler(), "PUT", "/interfaces/eth0/config", goodBody)
	if code != 502 || !strings.Contains(out["error"].(map[string]any)["message"].(string), "nope") {
		t.Errorf("apply: %d %v", code, out)
	}
	if b.reverted() != 1 || s.trial.status().Trial != nil {
		t.Errorf("reverts %d, status %+v", b.reverted(), s.trial.status())
	}
	if _, err := os.Stat(filepath.Join(dir, recordFile)); !os.IsNotExist(err) {
		t.Errorf("record remains: %v", err)
	}
}

// A failing revert must keep trying: it may be what restores the network.
func TestFailedRevertIsRetriedAndReported(t *testing.T) {
	b := &fakeBackend{failRevert: errors.New("busy")}
	s, dir := newService(t, b, time.Hour)
	h := s.Handler()
	call(t, h, "PUT", "/interfaces/eth0/config", goodBody)
	if code, _ := call(t, h, "POST", "/trial/revert", ""); code != 500 {
		t.Errorf("revert: %d", code)
	}
	status := s.trial.status()
	if status.Trial == nil || status.Trial.RevertError != "busy" {
		t.Errorf("status = %+v", status)
	}
	if _, err := os.Stat(filepath.Join(dir, recordFile)); err != nil {
		t.Errorf("record lost: %v", err)
	}
	b.mu.Lock()
	b.failRevert = nil
	b.mu.Unlock()
	if code, _ := call(t, h, "POST", "/trial/revert", ""); code != 200 {
		t.Errorf("second revert: %d", code)
	}
}

// A trial survives the panel: the next start waits out what is left, and
// reverts at once when the time has passed.
func TestTrialSurvivesRestart(t *testing.T) {
	b := &fakeBackend{}
	yes := true
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	first := New(Options{Runner: ipRunner{device: "lo"}, Backend: b, Installed: &yes, StateDir: dir, Window: 30 * time.Millisecond, Logger: logger})
	if code, out := call(t, first.Handler(), "PUT", "/interfaces/eth0/config", goodBody); code != 200 {
		t.Fatalf("apply: %d %v", code, out)
	}
	first.Close() // stops its timer; the record stays
	time.Sleep(50 * time.Millisecond)
	if b.reverted() != 0 {
		t.Fatal("a closed service reverted")
	}
	second := New(Options{Runner: ipRunner{device: "lo"}, Backend: b, Installed: &yes, StateDir: dir, Logger: logger})
	defer second.Close()
	deadline := time.Now().Add(2 * time.Second)
	for b.reverted() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if b.reverted() != 1 {
		t.Fatal("did not revert after the restart")
	}
}

func TestApplyRefusals(t *testing.T) {
	b := &fakeBackend{}
	s, _ := newService(t, b, time.Hour)
	h := s.Handler()
	for name, c := range map[string]struct {
		path, body string
		code       int
	}{
		"stale version":  {"/interfaces/eth0/config", `{"version":"old","config":{"dhcp4":true}}`, 409},
		"invalid config": {"/interfaces/eth0/config", `{"version":"v1","config":{"addresses":["nope"]}}`, 422},
		"other software": {"/interfaces/docker0/config", `{"version":"v1","config":{"dhcp4":true}}`, 409},
		"no such":        {"/interfaces/eth9/config", goodBody, 404},
		"bad name":       {"/interfaces/..%2Fetc/config", goodBody, 404},
		"bad json":       {"/interfaces/eth0/config", `{`, 400},
	} {
		if code, out := call(t, h, "PUT", c.path, c.body); code != c.code {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if b.applied != 0 {
		t.Errorf("applied %d refused changes", b.applied)
	}
	b.reason = "generated"
	if code, _ := call(t, h, "PUT", "/interfaces/eth0/config", goodBody); code != 409 {
		t.Errorf("uneditable: %d", code)
	}
}

func TestApplyNeedsStateDir(t *testing.T) {
	yes := true
	b := &fakeBackend{}
	s := New(Options{Runner: ipRunner{device: "lo"}, Backend: b, Installed: &yes, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if code, _ := call(t, s.Handler(), "PUT", "/interfaces/eth0/config", goodBody); code != 409 || b.applied != 0 {
		t.Errorf("code %d, applied %d", code, b.applied)
	}
}

func TestDownProtectsUplinkAndThisClient(t *testing.T) {
	b := &fakeBackend{}
	s, _ := newService(t, b, time.Hour)
	h := s.Handler()
	if code, _ := call(t, h, "POST", "/interfaces/eth0/down", ""); code != 409 {
		t.Errorf("uplink: %d", code)
	}
	if code, _ := call(t, h, "POST", "/interfaces/docker0/down", ""); code != 409 {
		t.Errorf("docker: %d", code)
	}
	if code, out := call(t, h, "POST", "/interfaces/eth1/down", ""); code != 200 {
		t.Errorf("eth1: %d %v", code, out)
	}
	if code, _ := call(t, h, "POST", "/interfaces/eth0/up", ""); code != 200 {
		t.Errorf("up: %d", code)
	}
	// Someone connected through eth1 cannot take it down either.
	yes := true
	viaEth1 := New(Options{Runner: ipRunner{device: "eth1"}, Backend: b, Installed: &yes, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	req := httptest.NewRequest("POST", "/interfaces/eth1/down", nil)
	req.RemoteAddr = "10.1.1.1:5555"
	rec := httptest.NewRecorder()
	viaEth1.Handler().ServeHTTP(rec, req)
	if rec.Code != 409 {
		t.Errorf("this client: %d", rec.Code)
	}
}

func TestRemoteIP(t *testing.T) {
	for _, c := range []struct{ remote, forwarded, want string }{
		{"10.0.0.9:1", "", "10.0.0.9"},
		{"10.0.0.9:1", "1.2.3.4", "10.0.0.9"}, // only a local proxy is believed
		{"127.0.0.1:1", "", ""},
		{"127.0.0.1:1", "6.6.6.6, 10.0.0.9", "10.0.0.9"},
		{"[::1]:1", "fd00::9", "fd00::9"},
		{"127.0.0.1:1", "127.0.0.1", ""},
		{"127.0.0.1:1", "; rm -rf", ""},
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = c.remote
		if c.forwarded != "" {
			req.Header.Set("X-Forwarded-For", c.forwarded)
		}
		if got := remoteIP(req); got != c.want {
			t.Errorf("%+v: got %q", c, got)
		}
	}
}

func TestToolsRefuseWhatCommandsWouldReadAsOptions(t *testing.T) {
	s, _ := newService(t, &fakeBackend{}, time.Hour)
	h := s.Handler()
	for _, target := range []string{"-f", "--help", "a b", "a;b", "", "$(x)", "-1.1.1.1"} {
		body, _ := json.Marshal(toolRequest{Tool: "ping", Target: target})
		if code, _ := call(t, h, "POST", "/tools/run", string(body)); code != 422 {
			t.Errorf("%q: %d", target, code)
		}
	}
	if code, _ := call(t, h, "POST", "/tools/run", `{"tool":"rm","target":"x"}`); code != 400 {
		t.Errorf("unknown tool: %d", code)
	}
	if code, _ := call(t, h, "POST", "/tools/run", `{"tool":"port","target":"x","port":70000}`); code != 422 {
		t.Errorf("bad port: %d", code)
	}
	for _, target := range []string{"example.com", "1.1.1.1", "fd00::1", "a-b_c.example."} {
		if !validTarget(target) {
			t.Errorf("%q refused", target)
		}
	}
}

func TestPortToolStreams(t *testing.T) {
	s, _ := newService(t, &fakeBackend{}, time.Hour)
	listener := httptest.NewServer(http.NotFoundHandler())
	defer listener.Close()
	_, port, _ := strings.Cut(strings.TrimPrefix(listener.URL, "http://"), ":")
	req := httptest.NewRequest("POST", "/tools/run", strings.NewReader(`{"tool":"port","target":"127.0.0.1","port":`+port+`}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "open") {
		t.Errorf("%d %q", rec.Code, rec.Body)
	}
}

func TestUnsupported(t *testing.T) {
	no := false
	s := New(Options{Installed: &no, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if s.Supported() {
		t.Error("supported")
	}
	if code, _ := call(t, s.Handler(), "GET", "/interfaces", ""); code != 503 {
		t.Errorf("interfaces: %d", code)
	}
	if code, _ := call(t, s.Handler(), "GET", "/info", ""); code != 200 {
		t.Errorf("info: %d", code)
	}
}
