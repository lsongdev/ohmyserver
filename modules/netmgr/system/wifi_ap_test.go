package system

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWifiAPNormalize(t *testing.T) {
	good := WifiAPRequest{SSID: "Home AP", Password: "password1", Address: "192.168.50.1/24"}
	r, err := good.Normalize()
	if err != nil || r.Band != "bg" || r.Address != "192.168.50.1/24" {
		t.Fatalf("%+v, %v", r, err)
	}
	if _, err := (WifiAPRequest{SSID: "x", Bridge: "br-lan", Band: "a", Channel: 36}).Normalize(); err != nil {
		t.Errorf("a bridged hotspot on 5 GHz: %v", err)
	}
	bad := map[string]struct {
		r     WifiAPRequest
		field string
	}{
		"no name":            {WifiAPRequest{Address: "192.168.50.1/24"}, "ssid"},
		"long name":          {WifiAPRequest{SSID: strings.Repeat("a", 33), Address: "192.168.50.1/24"}, "ssid"},
		"newline in name":    {WifiAPRequest{SSID: "a\nb", Address: "192.168.50.1/24"}, "ssid"},
		"short password":     {WifiAPRequest{SSID: "x", Password: "short", Address: "192.168.50.1/24"}, "password"},
		"non-ASCII password": {WifiAPRequest{SSID: "x", Password: "pässwörd1", Address: "192.168.50.1/24"}, "password"},
		"band":               {WifiAPRequest{SSID: "x", Band: "ac", Address: "192.168.50.1/24"}, "band"},
		"channel in band":    {WifiAPRequest{SSID: "x", Band: "bg", Channel: 36, Address: "192.168.50.1/24"}, "channel"},
		"channel 5 GHz":      {WifiAPRequest{SSID: "x", Band: "a", Channel: 6, Address: "192.168.50.1/24"}, "channel"},
		"neither":            {WifiAPRequest{SSID: "x"}, "address"},
		"both":               {WifiAPRequest{SSID: "x", Address: "192.168.50.1/24", Bridge: "br-lan"}, "bridge"},
		"network address":    {WifiAPRequest{SSID: "x", Address: "192.168.50.0/24"}, "address"},
		"no prefix":          {WifiAPRequest{SSID: "x", Address: "192.168.50.1"}, "address"},
		"IPv6":               {WifiAPRequest{SSID: "x", Address: "fd00::1/64"}, "address"},
		"bridge name":        {WifiAPRequest{SSID: "x", Bridge: "br;x"}, "bridge"},
	}
	for name, c := range bad {
		_, err := c.r.Normalize()
		if v, ok := err.(*ValidationError); !ok || v.Fields[c.field] == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestWifiAPKeyfile(t *testing.T) {
	uuid := wifiAPUUID("wlan0")
	plain := wifiAPKeyfile(uuid, "wlan0", WifiAPRequest{SSID: "Home AP", Password: "pass word", Band: "bg", Channel: 6, Address: "192.168.50.1/24"})
	for _, want := range []string{
		"id=Home AP", "uuid=" + uuid, "interface-name=wlan0", "autoconnect-priority=10", "mode=ap", "ssid=Home AP", "band=bg", "channel=6",
		"key-mgmt=wpa-psk", "proto=rsn", "pairwise=ccmp", "psk=pass word", "method=manual", "address1=192.168.50.1/24", "[ipv6]\nmethod=ignore",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("lacks %q:\n%s", want, plain)
		}
	}
	for _, no := range []string{"master=", "hidden", "tkip"} {
		if strings.Contains(plain, no) {
			t.Errorf("has %q:\n%s", no, plain)
		}
	}
	// A port of a bridge has no addresses of its own, and a channel left to
	// the radio is not written.
	bridged := wifiAPKeyfile(uuid, "wlan0", WifiAPRequest{SSID: "x", Password: "password1", Band: "a", Hidden: true, Bridge: "br-lan"})
	for _, want := range []string{"master=br-lan", "slave-type=bridge", "band=a", "hidden=true"} {
		if !strings.Contains(bridged, want) {
			t.Errorf("lacks %q:\n%s", want, bridged)
		}
	}
	for _, no := range []string{"[ipv4]", "[ipv6]", "channel="} {
		if strings.Contains(bridged, no) {
			t.Errorf("has %q:\n%s", no, bridged)
		}
	}
}

func TestWifiAPUUIDDiffersFromTheClientProfilesAndPerRadio(t *testing.T) {
	if wifiAPUUID("wlan0") == wifiAPUUID("wlan1") || wifiAPUUID("wlan0") == wifiUUID("wlan0") || wifiAPUUID("wlan0") != wifiAPUUID("wlan0") {
		t.Error("uuids collide or are unstable")
	}
}

func apShow(uuid string) string {
	return "nmcli -t -e no -f " + apFields + " connection show " + uuid
}

// apRunner is a radio joined to HomeNet (u-home), with a bridge br-lan. When
// withAP is set the hotspot exists on disk, and is up if active.
func apRunner(t *testing.T, dir string, withAP, active bool) *fakeRunner {
	t.Helper()
	uuid := wifiAPUUID("wlan0")
	r := &fakeRunner{answers: map[string]string{
		"nmcli -t -f UUID,DEVICE connection show --active":  "u-home:wlan0\nu-eth:eth0\n",
		"nmcli -t -e no -f UUID connection show":            "u-br\n",
		profKey("u-br"):                                     profile("br-lan", "bridge", "br-lan", "", ""),
		"nmcli -t -e no -f bridge.stp connection show u-br": "bridge.stp:no\n",
	}}
	if withAP {
		os.WriteFile(filepath.Join(dir, "ohmyserver-"+uuid+".nmconnection"),
			[]byte(wifiAPKeyfile(uuid, "wlan0", WifiAPRequest{SSID: "Home AP", Password: "oldpass-1", Band: "bg", Channel: 6, Address: "192.168.50.1/24"})), 0o600)
		r.answers[apShow(uuid)] = "802-11-wireless.ssid:Home AP\n802-11-wireless.band:bg\n802-11-wireless.channel:6\n802-11-wireless.hidden:no\nconnection.master:--\nipv4.addresses:192.168.50.1/24\n"
		if active {
			r.answers["nmcli -t -f UUID,DEVICE connection show --active"] = uuid + ":wlan0\nu-eth:eth0\n"
		}
	}
	return r
}

func TestWifiAPReadsOnlyWhatThisModuleMade(t *testing.T) {
	dir := t.TempDir()
	n := &NetworkManager{Run: apRunner(t, dir, false, false), ConnDir: dir}
	if ap, err := n.WifiAP(context.Background(), "wlan0"); ap != nil || err != nil {
		t.Errorf("a hotspot nobody made: %+v, %v", ap, err)
	}
	dir = t.TempDir()
	n = &NetworkManager{Run: apRunner(t, dir, true, true), ConnDir: dir}
	ap, err := n.WifiAP(context.Background(), "wlan0")
	if err != nil || ap == nil {
		t.Fatalf("%+v, %v", ap, err)
	}
	if ap.SSID != "Home AP" || ap.Band != "bg" || ap.Channel != 6 || ap.Hidden || ap.Address != "192.168.50.1/24" || ap.Bridge != "" || !ap.HasPassword || !ap.Active {
		t.Errorf("ap = %+v", ap)
	}
}

func TestPrepareWifiAPRefusals(t *testing.T) {
	dir := t.TempDir()
	n := &NetworkManager{Run: apRunner(t, dir, false, false), ConnDir: dir}
	_, err := n.PrepareWifiAP(context.Background(), "wlan0", WifiAPRequest{SSID: "x", Band: "bg", Address: "192.168.50.1/24"})
	if v, ok := err.(*ValidationError); !ok || v.Fields["password"] == "" {
		t.Errorf("no password: %v", err)
	}
	_, err = n.PrepareWifiAP(context.Background(), "wlan0", WifiAPRequest{SSID: "x", Password: "password1", Band: "bg", Bridge: "br-nope"})
	if v, ok := err.(*ValidationError); !ok || v.Fields["bridge"] == "" {
		t.Errorf("no such bridge: %v", err)
	}
	if _, err := n.PrepareWifiAP(context.Background(), "wlan0", WifiAPRequest{SSID: "x", Password: "password1", Band: "bg", Bridge: "br-lan"}); err != nil {
		t.Errorf("a bridge made here: %v", err)
	}
}

// A new hotspot is undone by deleting it, then bringing back the network the
// radio had joined, in that order.
func TestNewWifiAPUndoDeletesItThenBringsBackTheJoinedNetwork(t *testing.T) {
	dir := t.TempDir()
	req := WifiAPRequest{SSID: "Home AP", Password: "password1", Band: "bg", Address: "192.168.50.1/24"}
	run := apRunner(t, dir, false, false)
	n := &NetworkManager{Run: run, ConnDir: dir}
	undo, err := n.PrepareWifiAP(context.Background(), "wlan0", req)
	if err != nil || len(undo.Profiles) != 2 || undo.Profiles[0].UUID != "u-home" || !undo.Profiles[1].Created {
		t.Fatalf("%+v, %v", undo, err)
	}
	if err := n.SaveWifiAP(context.Background(), "wlan0", req); err != nil {
		t.Fatal(err)
	}
	uuid, file := wifiAPUUID("wlan0"), undo.Profiles[1].File
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v, %v", info, err)
	}
	run.calls = nil
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	want := []string{"nmcli connection delete " + uuid, "nmcli connection up u-home ifname wlan0"}
	if got := changes(run.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("the hotspot's file, with its password, was left behind")
	}
}

func TestSavingAWifiAPKeepsThePasswordOffCommandLinesAndKeepsTheOldOneWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	run := apRunner(t, dir, true, true)
	n := &NetworkManager{Run: run, ConnDir: dir}
	uuid := wifiAPUUID("wlan0")
	req := WifiAPRequest{SSID: "Renamed AP", Band: "bg", Channel: 11, Address: "192.168.50.1/24"} // no password: keep it
	undo, err := n.PrepareWifiAP(context.Background(), "wlan0", req)
	if err != nil {
		t.Fatal(err)
	}
	// An existing hotspot is copied for the revert, and the copy is removed when kept.
	last := undo.Profiles[len(undo.Profiles)-1]
	if last.Created || last.Backup != filepath.Join(dir, ".ohmyserver-"+uuid+".prev") || !last.Reup || !reflect.DeepEqual(undo.Cleanup, []string{last.Backup}) {
		t.Fatalf("undo = %+v", undo)
	}
	if err := n.SaveWifiAP(context.Background(), "wlan0", req); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(last.File)
	for _, want := range []string{"ssid=Renamed AP", "channel=11", "psk=oldpass-1"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	for _, call := range run.calls {
		if strings.Contains(call, "oldpass") {
			t.Errorf("the password is on a command line: %q", call)
		}
	}
	// It is hung up before it is dialed again, as a PPPoE connection is.
	want := []string{"nmcli connection load " + last.File, "nmcli connection down " + uuid, "nmcli --wait 45 connection up " + uuid}
	if got := changes(run.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
	run.calls = nil
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if old, _ := os.ReadFile(last.File); !strings.Contains(string(old), "ssid=Home AP") || !strings.Contains(string(old), "channel=6") {
		t.Errorf("not restored:\n%s", old)
	}
	if _, err := os.Stat(last.Backup); err == nil {
		t.Error("backup still there")
	}
}

// Editing a hotspot that was down starts it, so a revert stops it again
// before it puts the file back.
func TestEditingAStoppedWifiAPIsStoppedAgainByTheRevert(t *testing.T) {
	dir := t.TempDir()
	n := &NetworkManager{Run: apRunner(t, dir, true, false), ConnDir: dir}
	undo, err := n.PrepareWifiAP(context.Background(), "wlan0", WifiAPRequest{SSID: "x", Band: "bg", Address: "192.168.50.1/24"})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, p := range undo.Profiles {
		switch {
		case p.Down:
			kinds = append(kinds, "down")
		case p.Backup != "":
			kinds = append(kinds, "restore")
		default:
			kinds = append(kinds, "reactivate")
		}
	}
	// Reverting walks backwards: restore the file, stop it, then bring back the joined network.
	if !reflect.DeepEqual(kinds, []string{"reactivate", "down", "restore"}) {
		t.Errorf("undo order = %v", kinds)
	}
}

func TestStopWifiAPEndsItAndKeepsItFromStartingAtBoot(t *testing.T) {
	dir := t.TempDir()
	run := apRunner(t, dir, true, true)
	uuid := wifiAPUUID("wlan0")
	if err := (&NetworkManager{Run: run, ConnDir: dir}).StopWifiAP(context.Background(), "wlan0"); err != nil {
		t.Fatal(err)
	}
	want := []string{"nmcli connection down " + uuid, "nmcli connection modify " + uuid + " connection.autoconnect no"}
	if got := changes(run.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
}

// The hotspot is not a network to join: it does not appear among the saved ones.
func TestAHotspotIsNotListedAmongSavedNetworks(t *testing.T) {
	run := wifiRunner()
	run.answers["nmcli -t -f UUID,TYPE connection show"] = "u-home:802-11-wireless\nu-ap:802-11-wireless\n"
	run.answers[savedKey+"u-ap"] = "connection.id:Home AP\nconnection.interface-name:wlan0\nconnection.autoconnect:yes\n802-11-wireless.ssid:Home AP\n802-11-wireless-security.key-mgmt:wpa-psk\n802-11-wireless.mode:ap\n"
	saved, err := (&NetworkManager{Run: run}).WifiSaved(context.Background())
	if err != nil || len(saved) != 1 || saved[0].SSID != "HomeNet" {
		t.Errorf("%+v, %v", saved, err)
	}
}
