package api

import (
	"context"
	"encoding/json"
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

	"github.com/lsongdev/ohmyserver/modules/fwmgr/system"
)

// fakeEngine records every configuration it is asked to apply.
type fakeEngine struct {
	mu        sync.Mutex
	missing   bool
	applied   []system.Config
	failNext  error // for the next Apply only
	checkFail error
}

func (f *fakeEngine) Available() bool { return !f.missing }

func (f *fakeEngine) Check(context.Context, system.Config) error { return f.checkFail }

func (f *fakeEngine) Apply(_ context.Context, c system.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failNext; err != nil {
		f.failNext = nil
		return err
	}
	f.applied = append(f.applied, c)
	return nil
}

func (f *fakeEngine) Rules(context.Context) (string, error) { return "table inet ohmyserver {}", nil }

func (f *fakeEngine) last() system.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.applied) == 0 {
		return system.Config{}
	}
	return f.applied[len(f.applied)-1]
}

func (f *fakeEngine) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.applied)
}

func newService(t *testing.T, e *fakeEngine, dir string, window time.Duration) *Service {
	t.Helper()
	yes := true
	s := New(Options{Engine: e, Supported: &yes, StateDir: dir, Window: window, Runner: ipRunner{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(func() { s.Close() })
	return s
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

const goodBody = `{"enabled":true,"wan":["eth1"],"masquerade":true,"allow_ping":true,"allow":[{"proto":"tcp","port":443}]}`

func TestChangeIsOnTrialThenKept(t *testing.T) {
	e, dir := &fakeEngine{}, t.TempDir()
	h := newService(t, e, dir, time.Hour).Handler()
	if code, out := call(t, h, "PUT", "/config", goodBody); code != 200 || !e.last().Enabled {
		t.Fatalf("apply: %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, trialFile)); err != nil {
		t.Errorf("no trial record: %v", err)
	}
	// Until it is kept, the saved configuration is still the old one.
	if _, err := os.Stat(filepath.Join(dir, configFile)); err == nil {
		t.Error("saved before it was kept")
	}
	if code, _ := call(t, h, "PUT", "/config", goodBody); code != 409 {
		t.Errorf("second change during a trial: %d", code)
	}
	if code, _ := call(t, h, "POST", "/trial/keep", ""); code != 200 {
		t.Fatalf("keep: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, trialFile)); err == nil {
		t.Error("trial record left after keeping")
	}
	var saved system.Config
	data, _ := os.ReadFile(filepath.Join(dir, configFile))
	if json.Unmarshal(data, &saved) != nil || !saved.Enabled || saved.WAN[0] != "eth1" {
		t.Errorf("saved = %s", data)
	}
	if code, _ := call(t, h, "POST", "/trial/keep", ""); code != 409 {
		t.Errorf("keeping twice: %d", code)
	}
}

func TestRevertPutsBackWhatWasThere(t *testing.T) {
	e := &fakeEngine{}
	h := newService(t, e, t.TempDir(), time.Hour).Handler()
	call(t, h, "PUT", "/config", goodBody)
	if code, _ := call(t, h, "POST", "/trial/revert", ""); code != 200 || e.last().Enabled {
		t.Errorf("revert: %d, last %+v", code, e.last())
	}
	_, out := call(t, h, "GET", "/trial", "")
	if out["data"].(map[string]any)["trial"] != nil {
		t.Errorf("still on trial: %v", out)
	}
}

func TestChangeRevertsByItself(t *testing.T) {
	e := &fakeEngine{}
	s := newService(t, e, t.TempDir(), 40*time.Millisecond)
	call(t, s.Handler(), "PUT", "/config", goodBody)
	deadline := time.Now().Add(3 * time.Second)
	for e.last().Enabled && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if e.last().Enabled {
		t.Fatal("not reverted")
	}
	if st := s.status(); st.Trial != nil || st.Last == nil || st.Last.Result != "expired" {
		t.Errorf("status = %+v", st)
	}
}

// A change nft refuses is undone at once, and nothing is left on trial.
func TestFailedApplyRestoresAtOnce(t *testing.T) {
	e := &fakeEngine{}
	s := newService(t, e, t.TempDir(), time.Hour)
	// The first Apply is the start-up one; fail the one after it.
	e.failNext = &system.ExitError{Command: "nft", Code: 1, Output: "boom"}
	code, out := call(t, s.Handler(), "PUT", "/config", goodBody)
	if code != 502 || s.status().Trial != nil {
		t.Errorf("%d %v", code, out)
	}
}

func TestRefusals(t *testing.T) {
	e := &fakeEngine{}
	h := newService(t, e, t.TempDir(), time.Hour).Handler()
	before := e.count()
	for name, c := range map[string]struct {
		body string
		code int
	}{
		"no wan":        {`{"enabled":true}`, 422},
		"quote in name": {`{"enabled":true,"wan":["eth1\" drop"]}`, 422},
		"bad port":      {`{"enabled":true,"wan":["eth1"],"allow":[{"proto":"tcp","port":0}]}`, 422},
		"not JSON":      {`{"enabled":`, 400},
	} {
		if code, out := call(t, h, "PUT", "/config", c.body); code != c.code {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if e.count() != before {
		t.Error("a refused change was applied")
	}
	e.checkFail = &system.ExitError{Command: "nft", Code: 1, Output: "syntax error"}
	if code, _ := call(t, h, "PUT", "/config", goodBody); code != 502 || e.count() != before {
		t.Errorf("nft rejected the script: %d, applied %d", code, e.count()-before)
	}
	e.checkFail, e.missing = nil, true
	if code, _ := call(t, h, "PUT", "/config", goodBody); code != 409 {
		t.Errorf("without nft: %d", code)
	}
}

func TestChangesNeedAStateDir(t *testing.T) {
	h := newService(t, &fakeEngine{}, "", time.Hour).Handler()
	if code, _ := call(t, h, "PUT", "/config", goodBody); code != 409 {
		t.Errorf("%d", code)
	}
}

func TestPreviewShowsTheScriptAndChangesNothing(t *testing.T) {
	e := &fakeEngine{}
	h := newService(t, e, t.TempDir(), time.Hour).Handler()
	before := e.count()
	code, out := call(t, h, "POST", "/config/preview", goodBody)
	script, _ := out["data"].(map[string]any)["script"].(string)
	if code != 200 || !strings.Contains(script, `iifname "eth1" jump wan_in`) || e.count() != before {
		t.Errorf("%d %v", code, out)
	}
}

// The firewall is the panel's to keep: the kernel forgets it at every boot,
// so the saved configuration is applied when the panel starts.
func TestSavedConfigurationIsAppliedAtStart(t *testing.T) {
	dir := t.TempDir()
	data, _ := json.Marshal(system.Config{Enabled: true, WAN: []string{"eth1"}})
	os.WriteFile(filepath.Join(dir, configFile), data, 0o600)
	e := &fakeEngine{}
	newService(t, e, dir, time.Hour)
	if e.count() != 1 || !e.last().Enabled || e.last().WAN[0] != "eth1" {
		t.Errorf("applied %+v", e.applied)
	}
}

func TestNothingIsAppliedWithoutNft(t *testing.T) {
	e := &fakeEngine{missing: true}
	newService(t, e, t.TempDir(), time.Hour)
	if e.count() != 0 {
		t.Errorf("applied %+v", e.applied)
	}
}

func writeRecord(t *testing.T, dir string, rec record) {
	t.Helper()
	data, _ := json.Marshal(rec)
	if err := os.WriteFile(filepath.Join(dir, trialFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A change on trial carries on after the panel restarts: what was being
// tried is in effect again, and the time left is what remains.
func TestTrialSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	next := system.Config{Enabled: true, WAN: []string{"eth1"}}
	writeRecord(t, dir, record{Next: next, Deadline: time.Now().Add(time.Hour)})
	e := &fakeEngine{}
	s := newService(t, e, dir, time.Hour)
	if !e.last().Enabled || s.status().Trial == nil {
		t.Errorf("last %+v, status %+v", e.last(), s.status())
	}
	if code, _ := call(t, s.Handler(), "POST", "/trial/revert", ""); code != 200 || e.last().Enabled {
		t.Errorf("revert after restart: %d, %+v", code, e.last())
	}
}

// A trial whose time ran out while the panel was down is reverted without
// the change it tried being put in effect again, even for a moment.
func TestExpiredTrialRevertsWithoutApplyingItAgain(t *testing.T) {
	dir := t.TempDir()
	writeRecord(t, dir, record{Next: system.Config{Enabled: true, WAN: []string{"eth1"}}, Deadline: time.Now().Add(-time.Minute)})
	e := &fakeEngine{}
	s := newService(t, e, dir, time.Hour)
	for _, c := range e.applied {
		if c.Enabled {
			t.Errorf("applied what had run out of time: %+v", c)
		}
	}
	if st := s.status(); st.Trial != nil || st.Last == nil || st.Last.Result != "expired" {
		t.Errorf("status = %+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, trialFile)); err == nil {
		t.Error("record left behind")
	}
}

// The panel stopped between saving a change that was kept and removing its
// record: the record is not a reason to undo what was kept.
func TestKeptChangeIsNotRevertedAfterACrash(t *testing.T) {
	dir := t.TempDir()
	next := system.Config{Enabled: true, WAN: []string{"eth1"}}
	data, _ := json.Marshal(next)
	os.WriteFile(filepath.Join(dir, configFile), data, 0o600)
	writeRecord(t, dir, record{Next: next, Deadline: time.Now().Add(-time.Minute)})
	e := &fakeEngine{}
	s := newService(t, e, dir, time.Hour)
	if !e.last().Enabled || s.status().Trial != nil {
		t.Errorf("last %+v, status %+v", e.last(), s.status())
	}
	if _, err := os.Stat(filepath.Join(dir, trialFile)); err == nil {
		t.Error("stale record left behind")
	}
}

// ipRunner answers the iproute2 commands.
type ipRunner struct{}

func (ipRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, []byte, error) {
	call := name + " " + strings.Join(args, " ")
	switch {
	case strings.Contains(call, "link show"):
		return []byte(`[{"ifname":"lo","link_type":"loopback"},{"ifname":"eth0","link_type":"ether"},{"ifname":"eth1","link_type":"ether"}]`), nil, nil
	case strings.Contains(call, "-4 route show default"):
		return []byte(`[{"dst":"default","dev":"eth0"}]`), nil, nil
	case strings.Contains(call, "route get 192.168.8.50"):
		return []byte(`[{"dev":"eth1"}]`), nil, nil
	}
	return []byte("[]"), nil, nil
}

func TestInterfacesFlagTheUplinkAndTheClientsOwn(t *testing.T) {
	h := newService(t, &fakeEngine{}, t.TempDir(), time.Hour).Handler()
	req := httptest.NewRequest("GET", "/interfaces", nil)
	req.RemoteAddr = "192.168.8.50:51000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out struct {
		Data []Interface `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	got := map[string]Interface{}
	for _, i := range out.Data {
		got[i.Name] = i
	}
	if _, loopback := got["lo"]; loopback || len(got) != 2 || !got["eth0"].Uplink || got["eth0"].Client || !got["eth1"].Client || got["eth1"].Uplink {
		t.Errorf("interfaces = %+v", out.Data)
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	no := false
	s := New(Options{Supported: &no, Engine: &fakeEngine{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if s.Supported() {
		t.Error("supported")
	}
	if code, _ := call(t, s.Handler(), "GET", "/config", ""); code != 503 {
		t.Errorf("config: %d", code)
	}
	if code, _ := call(t, s.Handler(), "GET", "/info", ""); code != 200 {
		t.Errorf("info: %d", code)
	}
}
