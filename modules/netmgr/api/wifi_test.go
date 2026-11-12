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

// wifiFake is a network manager with one radio, wlan0, joined to HomeNet.
type wifiFake struct {
	fakeBackend
	connected []system.WifiRequest
	forgotten []string
	saved     []system.WifiSaved
	failJoin  error
}

func (w *wifiFake) WifiDevices(context.Context) ([]system.WifiDevice, error) {
	return []system.WifiDevice{{Name: "wlan0", State: "connected", SSID: "HomeNet"}}, nil
}

func (w *wifiFake) WifiScan(_ context.Context, device string, rescan bool) ([]system.WifiNetwork, error) {
	return []system.WifiNetwork{{SSID: "HomeNet", Signal: 80, Kind: "wpa-psk"}}, nil
}

func (w *wifiFake) WifiSaved(context.Context) ([]system.WifiSaved, error) { return w.saved, nil }

func (w *wifiFake) PrepareWifi(_ context.Context, device string, _ system.WifiRequest) (system.Undo, error) {
	return system.Undo{Backend: w.Name(), Link: device}, nil
}

func (w *wifiFake) ConnectWifi(_ context.Context, _ string, r system.WifiRequest) error {
	w.connected = append(w.connected, r)
	return w.failJoin
}

func (w *wifiFake) ForgetWifi(_ context.Context, uuid string) error {
	w.forgotten = append(w.forgotten, uuid)
	return nil
}

// radioRunner makes wlan0 the interface the default route leaves through.
type radioRunner struct{ ipRunner }

func (r radioRunner) Run(ctx context.Context, in string, name string, args ...string) ([]byte, []byte, error) {
	call := name + " " + strings.Join(args, " ")
	switch {
	case strings.Contains(call, "addr show"):
		return []byte(`[{"ifname":"wlan0","flags":["UP","LOWER_UP"],"mtu":1500,"operstate":"UP","link_type":"ether","addr_info":[]}]`), nil, nil
	case strings.Contains(call, "-4 -j route"):
		return []byte(`[{"dst":"default","gateway":"10.0.0.1","dev":"wlan0"}]`), nil, nil
	}
	return r.ipRunner.Run(ctx, in, name, args...)
}

func newWifiService(t *testing.T, log io.Writer) (*Service, *wifiFake) {
	t.Helper()
	yes := true
	w := &wifiFake{saved: []system.WifiSaved{{UUID: "aaaa-1111", Name: "Home", SSID: "HomeNet", Kind: "wpa-psk", Active: true}}}
	s := New(Options{Runner: radioRunner{ipRunner{device: "lo"}}, Backend: w, Installed: &yes, StateDir: t.TempDir(), Window: time.Hour,
		Logger: slog.New(slog.NewTextHandler(log, nil))})
	t.Cleanup(func() { s.Close() })
	return s, w
}

func TestWifiOverviewMarksTheRadioCarryingTheDefaultRoute(t *testing.T) {
	s, _ := newWifiService(t, io.Discard)
	code, out := call(t, s.Handler(), "GET", "/wifi", "")
	data, _ := out["data"].(map[string]any)
	devices, _ := data["devices"].([]any)
	if code != 200 || len(devices) != 1 || devices[0].(map[string]any)["protected"] != true || len(data["saved"].([]any)) != 1 {
		t.Errorf("%d %v", code, out)
	}
}

func TestWifiNetworksOnlyForARadioNetworkManagerLists(t *testing.T) {
	s, _ := newWifiService(t, io.Discard)
	if code, out := call(t, s.Handler(), "GET", "/wifi/wlan0/networks", ""); code != 200 || len(out["data"].([]any)) != 1 {
		t.Errorf("wlan0: %d %v", code, out)
	}
	for _, name := range []string{"eth0", "wlan9", "-x"} {
		if code, _ := call(t, s.Handler(), "GET", "/wifi/"+name+"/networks", ""); code != 404 {
			t.Errorf("%s: %d", name, code)
		}
	}
}

// Joining is on trial, and neither the response nor the log holds the password.
func TestWifiConnectIsOnTrialAndKeepsThePasswordOut(t *testing.T) {
	var log bytes.Buffer
	s, w := newWifiService(t, &log)
	code, out := call(t, s.Handler(), "PUT", "/wifi/wlan0/connection", `{"ssid":"Cafe","password":"s3cret-pass"}`)
	if code != 200 || len(w.connected) != 1 || w.connected[0].Security != "wpa-psk" {
		t.Fatalf("%d %v %+v", code, out, w.connected)
	}
	trial := out["data"].(map[string]any)["trial"].(map[string]any)
	if trial["subject"] != "wlan0" || trial["target"] != "Wi-Fi Cafe" {
		t.Errorf("trial = %v", trial)
	}
	if strings.Contains(log.String(), "s3cret") || strings.Contains(trialRecord(t, s), "s3cret") {
		t.Error("the password reached the log or the trial record")
	}
	if code, _ := call(t, s.Handler(), "POST", "/trial/revert", ""); code != 200 || w.reverted() != 1 {
		t.Errorf("revert: %d, %d", code, w.reverted())
	}
}

func TestWifiConnectRefusals(t *testing.T) {
	s, w := newWifiService(t, io.Discard)
	for name, c := range map[string]struct {
		path, body string
		code       int
	}{
		"short password": {"/wifi/wlan0/connection", `{"ssid":"Cafe","password":"short"}`, 422},
		"enterprise":     {"/wifi/wlan0/connection", `{"ssid":"Cafe","security":"wpa-eap"}`, 422},
		"unknown radio":  {"/wifi/wlan9/connection", `{"ssid":"Cafe"}`, 404},
		"bad JSON":       {"/wifi/wlan0/connection", `{"ssid":`, 400},
	} {
		if code, out := call(t, s.Handler(), "PUT", c.path, c.body); code != c.code {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if len(w.connected) != 0 {
		t.Errorf("joined %v", w.connected)
	}
}

// trialRecord is the file that survives a restart, as text.
func trialRecord(t *testing.T, s *Service) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.trial.dir, recordFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A join that fails is undone at once and leaves nothing on trial.
func TestWifiConnectFailureRestoresAtOnce(t *testing.T) {
	s, w := newWifiService(t, io.Discard)
	w.failJoin = &system.ExitError{Command: "nmcli", Code: 4, Output: "Connection activation failed: Secrets were required"}
	code, out := call(t, s.Handler(), "PUT", "/wifi/wlan0/connection", `{"ssid":"Cafe","password":"password1"}`)
	if code != 502 || w.reverted() != 1 || s.trial.status().Trial != nil {
		t.Errorf("%d %v, reverts %d, trial %+v", code, out, w.reverted(), s.trial.status().Trial)
	}
	if strings.Contains(trialRecord2(s), "password1") {
		t.Error("a failed join left the password on disk")
	}
}

func trialRecord2(s *Service) string {
	data, _ := os.ReadFile(filepath.Join(s.trial.dir, recordFile))
	return string(data)
}

func TestWifiForget(t *testing.T) {
	s, w := newWifiService(t, io.Discard)
	h := s.Handler()
	if code, _ := call(t, h, "DELETE", "/wifi/saved/nope-nope", ""); code != 404 {
		t.Errorf("unknown: %d", code)
	}
	if code, _ := call(t, h, "DELETE", "/wifi/saved/x;rm", ""); code != 404 {
		t.Errorf("malformed: %d", code)
	}
	// HomeNet is active on wlan0, which carries the default route.
	if code, out := call(t, h, "DELETE", "/wifi/saved/aaaa-1111", ""); code != 409 || len(w.forgotten) != 0 {
		t.Errorf("active on a protected radio: %d %v", code, out)
	}
	w.saved = append(w.saved, system.WifiSaved{UUID: "bbbb-2222", Name: "Old", SSID: "OldNet", Kind: "open"})
	if code, out := call(t, h, "DELETE", "/wifi/saved/bbbb-2222", ""); code != 200 || len(w.forgotten) != 1 || w.forgotten[0] != "bbbb-2222" {
		t.Errorf("idle network: %d %v %v", code, out, w.forgotten)
	}
}

func TestWifiForgetWaitsForATrial(t *testing.T) {
	s, w := newWifiService(t, io.Discard)
	w.saved = append(w.saved, system.WifiSaved{UUID: "bbbb-2222", SSID: "OldNet", Kind: "open"})
	h := s.Handler()
	call(t, h, "PUT", "/wifi/wlan0/connection", `{"ssid":"Cafe"}`)
	if code, _ := call(t, h, "DELETE", "/wifi/saved/bbbb-2222", ""); code != 409 || len(w.forgotten) != 0 {
		t.Errorf("during a trial: %d", code)
	}
}

func TestWifiIsRefusedWithoutNetworkManager(t *testing.T) {
	s, _ := newService(t, &fakeBackend{}, time.Hour)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/wifi", ""}, {"GET", "/wifi/wlan0/networks", ""},
		{"PUT", "/wifi/wlan0/connection", `{"ssid":"x"}`}, {"DELETE", "/wifi/saved/aaaa-1111", ""},
	} {
		if code, _ := call(t, s.Handler(), c.method, c.path, c.body); code != 409 {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
}
