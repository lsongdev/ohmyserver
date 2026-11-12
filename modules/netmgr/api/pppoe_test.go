package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// dialFake is a network manager that can dial, with one connection, Provider,
// that is up on ppp0.
type dialFake struct {
	fakeBackend
	conns    []system.PPPoE
	saved    []system.PPPoERequest
	dialed   []string
	hungup   []string
	forgot   []string
	ready    system.Readiness
	failSave error
}

func (d *dialFake) DetectFeatures(context.Context) system.Features {
	return system.Features{WiFi: []string{}, PPPoE: d.ready}
}

func (d *dialFake) PPPoEList(context.Context) ([]system.PPPoE, error) { return d.conns, nil }

func (d *dialFake) PreparePPPoE(_ context.Context, r system.PPPoERequest) (system.Undo, error) {
	return system.Undo{Backend: d.Name(), Link: r.Parent}, nil
}

func (d *dialFake) SavePPPoE(_ context.Context, r system.PPPoERequest) error {
	d.saved = append(d.saved, r)
	return d.failSave
}

func (d *dialFake) PrepareDial(_ context.Context, name string) (system.Undo, error) {
	return system.Undo{Backend: d.Name(), Link: name}, nil
}

func (d *dialFake) Dial(_ context.Context, name string) error {
	d.dialed = append(d.dialed, name)
	return nil
}

func (d *dialFake) Hangup(_ context.Context, name string) error {
	d.hungup = append(d.hungup, name)
	return nil
}

func (d *dialFake) ForgetPPPoE(_ context.Context, name string) error {
	d.forgot = append(d.forgot, name)
	return nil
}

func newDialService(t *testing.T, log io.Writer) (*Service, *dialFake) {
	t.Helper()
	yes := true
	d := &dialFake{
		ready: system.Readiness{Ready: true},
		conns: []system.PPPoE{
			{UUID: "u1", Name: "Provider", Parent: "eth1", Username: "user@isp", Managed: true, Active: true, Device: "ppp0"},
			{UUID: "u2", Name: "Spare", Parent: "eth1", Username: "x", Managed: true},
			{UUID: "u3", Name: "Manual", Parent: "eth1", Username: "y"},
		},
	}
	s := New(Options{Runner: pppRunner{}, Backend: d, Installed: &yes, StateDir: t.TempDir(), Window: time.Hour,
		Logger: slog.New(slog.NewTextHandler(log, nil))})
	t.Cleanup(func() { s.Close() })
	return s, d
}

// pppRunner has eth0 and eth1 as Ethernet, with the default route on ppp0.
type pppRunner struct{ ipRunner }

func (r pppRunner) Run(ctx context.Context, in string, name string, args ...string) ([]byte, []byte, error) {
	call := name + " " + strings.Join(args, " ")
	switch {
	case strings.Contains(call, "addr show"):
		return []byte(`[{"ifname":"eth0","flags":["UP","LOWER_UP"],"mtu":1500,"operstate":"UP","link_type":"ether","addr_info":[]},
		 {"ifname":"eth1","flags":["UP","LOWER_UP"],"mtu":1500,"operstate":"UP","link_type":"ether","addr_info":[]},
		 {"ifname":"ppp0","flags":["UP"],"mtu":1492,"operstate":"UNKNOWN","link_type":"ppp","addr_info":[]}]`), nil, nil
	case strings.Contains(call, "-4 -j route"):
		return []byte(`[{"dst":"default","dev":"ppp0"}]`), nil, nil
	}
	return r.ipRunner.Run(ctx, in, name, args...)
}

func TestPPPoEOverview(t *testing.T) {
	s, _ := newDialService(t, io.Discard)
	code, out := call(t, s.Handler(), "GET", "/pppoe", "")
	data, _ := out["data"].(map[string]any)
	conns, _ := data["connections"].([]any)
	if code != 200 || data["ready"] != true || len(conns) != 3 || len(data["parents"].([]any)) != 2 {
		t.Fatalf("%d %v", code, out)
	}
	byName := map[string]map[string]any{}
	for _, c := range conns {
		byName[c.(map[string]any)["name"].(string)] = c.(map[string]any)
	}
	if byName["Provider"]["protected"] != true || byName["Spare"]["protected"] != false {
		t.Errorf("protected: %v", byName)
	}
}

func TestPPPoESaveIsOnTrialAndKeepsThePasswordOut(t *testing.T) {
	var log bytes.Buffer
	s, d := newDialService(t, &log)
	code, out := call(t, s.Handler(), "PUT", "/pppoe/New%20Link",
		`{"parent":"eth1","username":"me@isp","password":"Zq7-dial-secret","name":"ignored"}`)
	if code != 200 || len(d.saved) != 1 {
		t.Fatalf("%d %v %+v", code, out, d.saved)
	}
	// The name is the one in the path, and defaults are filled in.
	if r := d.saved[0]; r.Name != "New Link" || r.MTU != 1492 || r.Metric != 50 {
		t.Errorf("request = %+v", r)
	}
	if trial := out["data"].(map[string]any)["trial"].(map[string]any); trial["target"] != "PPPoE New Link" {
		t.Errorf("trial = %v", trial)
	}
	rec, _ := os.ReadFile(filepath.Join(s.trial.dir, recordFile))
	if strings.Contains(log.String(), "dial-secret") || strings.Contains(string(rec), "dial-secret") {
		t.Error("the password reached the log or the trial record")
	}
}

func TestPPPoESaveRefusals(t *testing.T) {
	s, d := newDialService(t, io.Discard)
	for name, c := range map[string]struct {
		path, body string
		code       int
	}{
		"no user":         {"/pppoe/X", `{"parent":"eth1"}`, 422},
		"parent is a ppp": {"/pppoe/X", `{"parent":"ppp0","username":"u","password":"p"}`, 422},
		"unknown parent":  {"/pppoe/X", `{"parent":"nope","username":"u","password":"p"}`, 422},
		"bad name":        {"/pppoe/%0A", `{"parent":"eth1","username":"u","password":"p"}`, 422},
		"bad JSON":        {"/pppoe/X", `{"parent":`, 400},
	} {
		if code, out := call(t, s.Handler(), "PUT", c.path, c.body); code != c.code {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if len(d.saved) != 0 {
		t.Errorf("saved %v", d.saved)
	}
}

func TestPPPoESaveNeedsWhatDialingNeeds(t *testing.T) {
	s, d := newDialService(t, io.Discard)
	d.ready = system.Readiness{Missing: []string{"pppd"}}
	code, out := call(t, s.Handler(), "PUT", "/pppoe/X", `{"parent":"eth1","username":"u","password":"p"}`)
	if code != 409 || !strings.Contains(out["error"].(map[string]any)["message"].(string), "pppd") || len(d.saved) != 0 {
		t.Errorf("%d %v", code, out)
	}
}

// A login that fails is undone at once and leaves nothing on trial.
func TestPPPoEFailedLoginRestoresAtOnce(t *testing.T) {
	s, d := newDialService(t, io.Discard)
	d.failSave = &system.ExitError{Command: "nmcli", Code: 4, Output: "Connection activation failed: PPP failed"}
	code, _ := call(t, s.Handler(), "PUT", "/pppoe/X", `{"parent":"eth1","username":"u","password":"p"}`)
	if code != 502 || d.reverted() != 1 || s.trial.status().Trial != nil {
		t.Errorf("%d, reverts %d, trial %+v", code, d.reverted(), s.trial.status().Trial)
	}
}

func TestPPPoEDial(t *testing.T) {
	s, d := newDialService(t, io.Discard)
	h := s.Handler()
	if code, _ := call(t, h, "POST", "/pppoe/Provider/dial", ""); code != 409 || len(d.dialed) != 0 {
		t.Errorf("already up: %d", code)
	}
	if code, _ := call(t, h, "POST", "/pppoe/Nope/dial", ""); code != 404 {
		t.Errorf("unknown: %d", code)
	}
	if code, out := call(t, h, "POST", "/pppoe/Spare/dial", ""); code != 200 || len(d.dialed) != 1 {
		t.Fatalf("dial: %d %v", code, out)
	}
	if code, _ := call(t, h, "POST", "/trial/revert", ""); code != 200 || d.reverted() != 1 {
		t.Errorf("revert: %d", code)
	}
}

func TestPPPoEHangupAndForgetProtections(t *testing.T) {
	s, d := newDialService(t, io.Discard)
	h := s.Handler()
	// Provider carries the default route.
	if code, _ := call(t, h, "POST", "/pppoe/Provider/hangup", ""); code != 409 || len(d.hungup) != 0 {
		t.Errorf("hangup of the uplink: %d", code)
	}
	if code, _ := call(t, h, "DELETE", "/pppoe/Provider", ""); code != 409 || len(d.forgot) != 0 {
		t.Errorf("forget the uplink: %d", code)
	}
	if code, _ := call(t, h, "DELETE", "/pppoe/Manual", ""); code != 409 || len(d.forgot) != 0 {
		t.Errorf("forget one made another way: %d", code)
	}
	if code, _ := call(t, h, "DELETE", "/pppoe/Spare", ""); code != 200 || len(d.forgot) != 1 || d.forgot[0] != "Spare" {
		t.Errorf("forget Spare: %d %v", code, d.forgot)
	}
	if code, _ := call(t, h, "POST", "/pppoe/Spare/hangup", ""); code != 200 || len(d.hungup) != 1 {
		t.Errorf("hangup Spare: %d", code)
	}
}

func TestPPPoEForgetWaitsForATrial(t *testing.T) {
	s, d := newDialService(t, io.Discard)
	h := s.Handler()
	call(t, h, "POST", "/pppoe/Spare/dial", "")
	if code, _ := call(t, h, "DELETE", "/pppoe/Spare", ""); code != 409 || len(d.forgot) != 0 {
		t.Errorf("during a trial: %d", code)
	}
}

func TestPPPoEIsRefusedWithoutNetworkManager(t *testing.T) {
	s, _ := newService(t, &fakeBackend{}, time.Hour)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/pppoe", ""}, {"PUT", "/pppoe/X", `{"parent":"eth1","username":"u","password":"p"}`},
		{"POST", "/pppoe/X/dial", ""}, {"POST", "/pppoe/X/hangup", ""}, {"DELETE", "/pppoe/X", ""},
	} {
		if code, _ := call(t, s.Handler(), c.method, c.path, c.body); code != 409 {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
}
