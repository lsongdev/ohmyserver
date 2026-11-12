package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// apFake is a network manager with one radio, wlan0, that can serve a network.
type apFake struct {
	wifiFake
	ap      *system.WifiAP
	saved   []system.WifiAPRequest
	stopped []string
	capable bool
	failAP  error
}

func (a *apFake) WifiDevices(context.Context) ([]system.WifiDevice, error) {
	return []system.WifiDevice{{Name: "wlan0", State: "connected", SSID: "HomeNet", AP: a.capable}}, nil
}

func (a *apFake) WifiAP(context.Context, string) (*system.WifiAP, error) { return a.ap, nil }

func (a *apFake) PrepareWifiAP(_ context.Context, device string, _ system.WifiAPRequest) (system.Undo, error) {
	return system.Undo{Backend: a.Name(), Link: device}, nil
}

func (a *apFake) SaveWifiAP(_ context.Context, _ string, r system.WifiAPRequest) error {
	a.saved = append(a.saved, r)
	return a.failAP
}

func (a *apFake) StopWifiAP(_ context.Context, device string) error {
	a.stopped = append(a.stopped, device)
	return nil
}

func (a *apFake) Bridges(context.Context) ([]system.Bridge, error) {
	return []system.Bridge{{Name: "br-lan"}}, nil
}
func (a *apFake) Bridge(context.Context, string) (*system.Bridge, error) { return nil, nil }
func (a *apFake) PrepareBridge(context.Context, string, system.BridgeConfig) (system.Undo, error) {
	return system.Undo{}, nil
}
func (a *apFake) ApplyBridge(context.Context, string, system.BridgeConfig) error { return nil }
func (a *apFake) PrepareDeleteBridge(context.Context, string) (system.Undo, error) {
	return system.Undo{}, nil
}
func (a *apFake) DeleteBridge(context.Context, string) error { return nil }

func newAPService(t *testing.T, log io.Writer) (*Service, *apFake) {
	t.Helper()
	yes := true
	a := &apFake{capable: true}
	s := New(Options{Runner: radioRunner{ipRunner{device: "lo"}}, Backend: a, Installed: &yes, StateDir: t.TempDir(), Window: time.Hour,
		Logger: slog.New(slog.NewTextHandler(log, nil))})
	t.Cleanup(func() { s.Close() })
	return s, a
}

const apBody = `{"ssid":"Home AP","password":"Zq7-hotspot-secret","band":"bg","channel":6,"address":"192.168.50.1/24"}`

func TestHotspotGetReportsCapabilityBridgesAndProtection(t *testing.T) {
	s, a := newAPService(t, io.Discard)
	a.ap = &system.WifiAP{Device: "wlan0", SSID: "Home AP", HasPassword: true}
	code, out := call(t, s.Handler(), "GET", "/wifi/wlan0/hotspot", "")
	data, _ := out["data"].(map[string]any)
	if code != 200 || data["capable"] != true || data["hotspot"].(map[string]any)["ssid"] != "Home AP" || data["protected"] != true ||
		len(data["bridges"].([]any)) != 1 {
		t.Errorf("%d %v", code, out)
	}
	if code, _ := call(t, s.Handler(), "GET", "/wifi/eth0/hotspot", ""); code != 404 {
		t.Errorf("not a radio: %d", code)
	}
	if _, out := call(t, s.Handler(), "GET", "/wifi/wlan0/hotspot", ""); out["data"].(map[string]any)["hotspot"] == nil {
		t.Error("hotspot missing")
	}
}

func TestHotspotSaveIsOnTrialAndKeepsThePasswordOut(t *testing.T) {
	var log bytes.Buffer
	s, a := newAPService(t, &log)
	code, out := call(t, s.Handler(), "PUT", "/wifi/wlan0/hotspot", apBody)
	if code != 200 || len(a.saved) != 1 || a.saved[0].SSID != "Home AP" {
		t.Fatalf("%d %v %+v", code, out, a.saved)
	}
	if trial := out["data"].(map[string]any)["trial"].(map[string]any); trial["subject"] != "wlan0" || trial["target"] != "Hotspot Home AP" {
		t.Errorf("trial = %v", trial)
	}
	if strings.Contains(log.String(), "hotspot-secret") || strings.Contains(trialRecord(t, s), "hotspot-secret") {
		t.Error("the password reached the log or the trial record")
	}
	if code, _ := call(t, s.Handler(), "POST", "/trial/revert", ""); code != 200 || a.reverted() != 1 {
		t.Errorf("revert: %d, %d", code, a.reverted())
	}
}

func TestHotspotSaveRefusals(t *testing.T) {
	s, a := newAPService(t, io.Discard)
	for name, c := range map[string]struct {
		path, body string
		code       int
	}{
		"no address or bridge": {"/wifi/wlan0/hotspot", `{"ssid":"x","password":"password1"}`, 422},
		"both":                 {"/wifi/wlan0/hotspot", `{"ssid":"x","password":"password1","address":"192.168.50.1/24","bridge":"br-lan"}`, 422},
		"channel":              {"/wifi/wlan0/hotspot", `{"ssid":"x","password":"password1","band":"bg","channel":40,"address":"192.168.50.1/24"}`, 422},
		"unknown radio":        {"/wifi/wlan9/hotspot", apBody, 404},
		"bad JSON":             {"/wifi/wlan0/hotspot", `{"ssid":`, 400},
	} {
		if code, out := call(t, s.Handler(), "PUT", c.path, c.body); code != c.code {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	a.capable = false
	if code, _ := call(t, s.Handler(), "PUT", "/wifi/wlan0/hotspot", apBody); code != 409 {
		t.Errorf("a radio with no access point mode: %d", code)
	}
	if len(a.saved) != 0 {
		t.Errorf("saved %v", a.saved)
	}
}

func TestHotspotFailedStartRestoresAtOnce(t *testing.T) {
	s, a := newAPService(t, io.Discard)
	a.failAP = &system.ExitError{Command: "nmcli", Code: 4, Output: "Connection activation failed"}
	code, _ := call(t, s.Handler(), "PUT", "/wifi/wlan0/hotspot", apBody)
	if code != 502 || a.reverted() != 1 || s.trial.status().Trial != nil {
		t.Errorf("%d, reverts %d, trial %+v", code, a.reverted(), s.trial.status().Trial)
	}
}

func TestHotspotStop(t *testing.T) {
	s, a := newAPService(t, io.Discard)
	h := s.Handler()
	if code, _ := call(t, h, "POST", "/wifi/wlan0/hotspot/stop", ""); code != 404 {
		t.Errorf("no hotspot: %d", code)
	}
	a.ap = &system.WifiAP{Device: "wlan0", SSID: "Home AP", Active: true}
	if code, _ := call(t, h, "POST", "/wifi/wlan0/hotspot/stop", ""); code != 200 || len(a.stopped) != 1 {
		t.Fatalf("stop: %d %v", code, a.stopped)
	}
	// While a change is on trial, nothing else is stopped.
	call(t, h, "PUT", "/wifi/wlan0/hotspot", apBody)
	if code, _ := call(t, h, "POST", "/wifi/wlan0/hotspot/stop", ""); code != 409 || len(a.stopped) != 1 {
		t.Errorf("during a trial: %d", code)
	}
}

func TestHotspotsAreRefusedWithoutNetworkManager(t *testing.T) {
	s, _ := newService(t, &fakeBackend{}, time.Hour)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/wifi/wlan0/hotspot", ""}, {"PUT", "/wifi/wlan0/hotspot", apBody}, {"POST", "/wifi/wlan0/hotspot/stop", ""},
	} {
		if code, _ := call(t, s.Handler(), c.method, c.path, c.body); code != 409 {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
}
