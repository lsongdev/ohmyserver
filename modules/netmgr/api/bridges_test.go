package api

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// bridgeFake keeps bridges in memory, as a network manager that makes them.
type bridgeFake struct {
	fakeBackend
	bridges          map[string]*system.Bridge
	applied, deleted int
}

func (b *bridgeFake) Bridges(context.Context) ([]system.Bridge, error) {
	list := []system.Bridge{}
	for _, v := range b.bridges {
		list = append(list, *v)
	}
	return list, nil
}

func (b *bridgeFake) Bridge(_ context.Context, name string) (*system.Bridge, error) {
	if v, ok := b.bridges[name]; ok {
		c := *v
		return &c, nil
	}
	return nil, nil
}

func (b *bridgeFake) PrepareBridge(_ context.Context, name string, _ system.BridgeConfig) (system.Undo, error) {
	return system.Undo{Backend: b.Name(), Link: name}, nil
}

func (b *bridgeFake) ApplyBridge(_ context.Context, name string, c system.BridgeConfig) error {
	b.applied++
	b.bridges[name] = &system.Bridge{Name: name, STP: c.STP, Ports: c.Ports, Version: "v2"}
	return nil
}

func (b *bridgeFake) PrepareDeleteBridge(_ context.Context, name string) (system.Undo, error) {
	return system.Undo{Backend: b.Name(), Link: name}, nil
}

func (b *bridgeFake) DeleteBridge(_ context.Context, name string) error {
	b.deleted++
	delete(b.bridges, name)
	return nil
}

func newBridgeService(t *testing.T) (*Service, *bridgeFake) {
	t.Helper()
	yes := true
	b := &bridgeFake{bridges: map[string]*system.Bridge{
		"br-lan": {Name: "br-lan", Ports: []string{"eth0"}, Version: "v1"},
	}}
	s := New(Options{Runner: ipRunner{device: "lo"}, Backend: b, Installed: &yes, StateDir: t.TempDir(), Window: time.Hour,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(func() { s.Close() })
	return s, b
}

func TestBridgeCreateIsOnTrialAndRevertable(t *testing.T) {
	s, b := newBridgeService(t)
	h := s.Handler()
	code, out := call(t, h, "PUT", "/bridges/br-new", `{"version":"","config":{"stp":true,"ports":["eth1"]}}`)
	if code != 200 || b.applied != 1 {
		t.Fatalf("create: %d %v", code, out)
	}
	trial := out["data"].(map[string]any)["trial"].(map[string]any)
	if trial["subject"] != "br-new" || trial["target"] != "Bridge br-new" {
		t.Errorf("trial = %v", trial)
	}
	if code, _ := call(t, h, "POST", "/trial/revert", ""); code != 200 || b.reverted() != 1 {
		t.Errorf("revert: %d, %d reverts", code, b.reverted())
	}
}

func TestBridgeRefusals(t *testing.T) {
	s, b := newBridgeService(t)
	h := s.Handler()
	for name, c := range map[string]struct{ path, body string }{
		"port is not Ethernet": {"/bridges/br-new", `{"version":"","config":{"ports":["docker0"]}}`},
		"port does not exist":  {"/bridges/br-new", `{"version":"","config":{"ports":["nope"]}}`},
		"name is another's":    {"/bridges/docker0", `{"version":"","config":{}}`},
		"name is an interface": {"/bridges/eth1", `{"version":"","config":{}}`},
		"name is too long":     {"/bridges/a-very-long-bridge-name", `{"version":"","config":{}}`},
		"port with a command":  {"/bridges/br-new", `{"version":"","config":{"ports":["eth1;rm"]}}`},
	} {
		if code, out := call(t, h, "PUT", c.path, c.body); code != 422 {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if b.applied != 0 || s.trial.status().Trial != nil {
		t.Errorf("a refused bridge changed something: %d applied", b.applied)
	}
}

func TestBridgeVersions(t *testing.T) {
	s, b := newBridgeService(t)
	h := s.Handler()
	// An existing bridge needs the version it was read at; a new one needs none.
	for name, c := range map[string]struct{ path, body string }{
		"stale version":           {"/bridges/br-lan", `{"version":"old","config":{"ports":["eth1"]}}`},
		"no version for existing": {"/bridges/br-lan", `{"version":"","config":{"ports":["eth1"]}}`},
		"version for a new one":   {"/bridges/br-new", `{"version":"v1","config":{"ports":["eth1"]}}`},
	} {
		if code, out := call(t, h, "PUT", c.path, c.body); code != 409 {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if b.applied != 0 {
		t.Fatalf("applied %d", b.applied)
	}
	if code, out := call(t, h, "PUT", "/bridges/br-lan", `{"version":"v1","config":{"ports":["eth1"]}}`); code != 200 {
		t.Errorf("current version: %d %v", code, out)
	}
}

func TestBridgeDelete(t *testing.T) {
	s, b := newBridgeService(t)
	h := s.Handler()
	if code, _ := call(t, h, "DELETE", "/bridges/br-lan?version=old", ""); code != 409 {
		t.Errorf("stale: %d", code)
	}
	if code, _ := call(t, h, "DELETE", "/bridges/nope?version=v1", ""); code != 404 {
		t.Errorf("unknown: %d", code)
	}
	if code, out := call(t, h, "DELETE", "/bridges/br-lan?version=v1", ""); code != 200 || b.deleted != 1 {
		t.Fatalf("delete: %d %v", code, out)
	}
	// It is on trial like any change, and can be undone.
	if code, _ := call(t, h, "POST", "/trial/revert", ""); code != 200 || b.reverted() != 1 {
		t.Errorf("revert: %d, %d", code, b.reverted())
	}
}

func TestBridgesAreRefusedWithoutNetworkManager(t *testing.T) {
	s, _ := newService(t, &fakeBackend{}, time.Hour)
	h := s.Handler()
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/bridges", ""}, {"GET", "/bridges/br-lan", ""},
		{"PUT", "/bridges/br-lan", `{"version":"","config":{}}`}, {"DELETE", "/bridges/br-lan?version=v1", ""},
	} {
		if code, _ := call(t, h, c.method, c.path, c.body); code != 409 {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
}

func TestBridgesList(t *testing.T) {
	s, _ := newBridgeService(t)
	code, out := call(t, s.Handler(), "GET", "/bridges", "")
	list, _ := out["data"].([]any)
	if code != 200 || len(list) != 1 || list[0].(map[string]any)["name"] != "br-lan" {
		t.Errorf("%d %v", code, out)
	}
	if code, _ := call(t, s.Handler(), "GET", "/bridges/missing", ""); code != 404 {
		t.Errorf("missing: %d", code)
	}
}
