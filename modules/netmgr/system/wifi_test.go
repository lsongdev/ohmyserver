package system

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitTerseUnescapes(t *testing.T) {
	got := splitTerse(`*:Caf\: 5G:80:36:5180 MHz:WPA2 WPA3`)
	want := []string{"*", "Caf: 5G", "80", "36", "5180 MHz", "WPA2 WPA3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := splitTerse(`a\\b:c`); !reflect.DeepEqual(got, []string{`a\b`, "c"}) {
		t.Errorf("backslash: %q", got)
	}
}

const scanText = `:HomeNet:55:6:2437 MHz:WPA2
*:HomeNet:81:36:5180 MHz:WPA2
::40:1:2412 MHz:WPA2
:Cafe\: Guest:70:11:2462 MHz:--
:Office:60:1:2412 MHz:WPA2 802.1X
:Legacy:30:6:2437 MHz:WEP
:Modern:65:149:5745 MHz:WPA3
:Mixed:64:44:5220 MHz:WPA2 WPA3
`

// A name heard from several access points is shown once, at its strongest,
// and keeps being marked in use. Hidden networks have no name to list.
func TestParseScan(t *testing.T) {
	list := parseScan(scanText)
	byName := map[string]WifiNetwork{}
	var order []string
	for _, n := range list {
		byName[n.SSID] = n
		order = append(order, n.SSID)
	}
	if len(list) != 6 {
		t.Fatalf("%d networks: %v", len(list), order)
	}
	if home := byName["HomeNet"]; home.Signal != 81 || home.Band != "5 GHz" || home.Channel != 36 || !home.InUse || home.Kind != "wpa-psk" {
		t.Errorf("HomeNet = %+v", home)
	}
	if !reflect.DeepEqual(order[:2], []string{"HomeNet", "Cafe: Guest"}) {
		t.Errorf("not strongest first: %v", order)
	}
	for name, kind := range map[string]string{"Cafe: Guest": "open", "Office": "unsupported", "Legacy": "unsupported", "Modern": "sae", "Mixed": "wpa-psk"} {
		if byName[name].Kind != kind {
			t.Errorf("%s kind = %q, want %q", name, byName[name].Kind, kind)
		}
	}
}

func TestWifiBand(t *testing.T) {
	for mhz, want := range map[int]string{2412: "2.4 GHz", 5180: "5 GHz", 5955: "6 GHz", 0: ""} {
		if got := wifiBand(mhz); got != want {
			t.Errorf("%d MHz = %q, want %q", mhz, got, want)
		}
	}
}

func TestWifiRequestNormalize(t *testing.T) {
	bad := map[string]struct {
		r     WifiRequest
		field string
	}{
		"empty name":       {WifiRequest{}, "ssid"},
		"long name":        {WifiRequest{SSID: strings.Repeat("a", 33)}, "ssid"},
		"newline in name":  {WifiRequest{SSID: "a\nb"}, "ssid"},
		"short password":   {WifiRequest{SSID: "x", Password: "short"}, "password"},
		"long password":    {WifiRequest{SSID: "x", Password: strings.Repeat("g", 64)}, "password"},
		"non-ASCII":        {WifiRequest{SSID: "x", Password: "pässwörd1"}, "password"},
		"open with secret": {WifiRequest{SSID: "x", Security: "open", Password: "password1"}, "password"},
		"enterprise":       {WifiRequest{SSID: "x", Security: "wpa-eap"}, "security"},
	}
	for name, c := range bad {
		_, err := c.r.Normalize()
		var v *ValidationError
		if err == nil || !asValidation(err, &v) || v.Fields[c.field] == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	good := map[string]WifiRequest{
		"open by default": {SSID: "Cafe"},
		"psk by password": {SSID: "Home", Password: "correct horse"},
		"hex key":         {SSID: "Home", Security: "wpa-psk", Password: strings.Repeat("ab", 32)},
		"sae":             {SSID: "Modern", Security: "sae", Password: "password1"},
		"name with colon": {SSID: "Cafe: Guest"},
	}
	for name, r := range good {
		if _, err := r.Normalize(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if r, _ := (WifiRequest{SSID: "x", Password: "password1"}).Normalize(); r.Security != "wpa-psk" {
		t.Errorf("security = %q", r.Security)
	}
}

func asValidation(err error, target **ValidationError) bool {
	v, ok := err.(*ValidationError)
	*target = v
	return ok
}

const savedKey = "nmcli -t -e no -f connection.id,connection.interface-name,connection.autoconnect,802-11-wireless.ssid,802-11-wireless-security.key-mgmt,802-11-wireless.mode connection show "

// wifiRunner is a machine with a wired port, a radio joined to HomeNet, and
// the saved network HomeNet.
func wifiRunner() *fakeRunner {
	return &fakeRunner{answers: map[string]string{
		"nmcli -t -f UUID,TYPE connection show":            "u-eth:802-3-ethernet\nu-home:802-11-wireless\n",
		"nmcli -t -f UUID,DEVICE connection show --active": "u-eth:eth0\nu-home:wlan0\n",
		savedKey + "u-home":                                "connection.id:Home\nconnection.interface-name:\nconnection.autoconnect:yes\n802-11-wireless.ssid:HomeNet\n802-11-wireless-security.key-mgmt:wpa-psk\n",
		"nmcli -t -f DEVICE,TYPE,STATE device":             "eth0:ethernet:connected\nwlan0:wifi:connected\np2p-dev-wlan0:wifi-p2p:disconnected\n",
		"nmcli -t -f IN-USE,SSID,SIGNAL,CHAN,FREQ,SECURITY device wifi list ifname wlan0 --rescan no": scanText,
	}}
}

func TestWifiSavedDevicesAndScanJoinTheSavedNetwork(t *testing.T) {
	n := &NetworkManager{Run: wifiRunner()}
	saved, err := n.WifiSaved(context.Background())
	if err != nil || len(saved) != 1 {
		t.Fatalf("%+v, %v", saved, err)
	}
	if s := saved[0]; s.UUID != "u-home" || s.SSID != "HomeNet" || s.Kind != "wpa-psk" || !s.Autoconnect || !s.Active {
		t.Errorf("saved = %+v", s)
	}
	devices, _ := n.WifiDevices(context.Background())
	if len(devices) != 1 || devices[0].Name != "wlan0" || devices[0].SSID != "HomeNet" {
		t.Errorf("devices = %+v", devices)
	}
	networks, _ := n.WifiScan(context.Background(), "wlan0", false)
	for _, w := range networks {
		if (w.SSID == "HomeNet") != (w.Saved == "u-home") {
			t.Errorf("%s saved = %q", w.SSID, w.Saved)
		}
	}
}

func TestPrepareWifiRefusesSilentChangesToASavedNetwork(t *testing.T) {
	n := &NetworkManager{Run: wifiRunner()}
	_, err := n.PrepareWifi(context.Background(), "wlan0", WifiRequest{SSID: "HomeNet", Security: "wpa-psk", Password: "another one"})
	if v, ok := err.(*ValidationError); !ok || v.Fields["password"] == "" {
		t.Errorf("new password for a saved network: %v", err)
	}
	_, err = n.PrepareWifi(context.Background(), "wlan0", WifiRequest{SSID: "NewNet", Security: "wpa-psk"})
	if v, ok := err.(*ValidationError); !ok || v.Fields["password"] == "" {
		t.Errorf("secured network without a password: %v", err)
	}
}

// What is active on the device comes back after the new connection is
// deleted, in that order.
func TestWifiUndoDeletesWhatItMadeThenBringsBackWhatWasActive(t *testing.T) {
	r := WifiRequest{SSID: "NewNet", Security: "wpa-psk", Password: "password1"}
	undo, err := (&NetworkManager{Run: wifiRunner()}).PrepareWifi(context.Background(), "wlan0", r)
	if err != nil || len(undo.Profiles) != 2 {
		t.Fatalf("%+v, %v", undo, err)
	}
	uuid := wifiUUID("NewNet")
	if p := undo.Profiles[1]; !p.Created || p.Name != uuid {
		t.Errorf("created = %+v", p)
	}
	run := wifiRunner()
	if err := (&NetworkManager{Run: run}).Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	want := []string{"nmcli connection delete " + uuid, "nmcli connection up u-home ifname wlan0"}
	if got := changes(run.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
}

// Joining the network that is already active needs no undo.
func TestWifiUndoIsEmptyForTheNetworkAlreadyActive(t *testing.T) {
	undo, err := (&NetworkManager{Run: wifiRunner()}).PrepareWifi(context.Background(), "wlan0", WifiRequest{SSID: "HomeNet", Security: "wpa-psk"})
	if err != nil || len(undo.Profiles) != 0 {
		t.Errorf("%+v, %v", undo, err)
	}
}

func readbackKey(uuid string) string {
	return "nmcli -g 802-11-wireless.ssid -e no connection show " + uuid
}

func TestConnectWifiKeepsThePasswordOffCommandLines(t *testing.T) {
	dir := t.TempDir()
	uuid := wifiUUID("NewNet")
	run := wifiRunner()
	run.answers[readbackKey(uuid)] = "NewNet\n"
	n := &NetworkManager{Run: run, ConnDir: dir}
	if err := n.ConnectWifi(context.Background(), "wlan0", WifiRequest{SSID: "NewNet", Security: "wpa-psk", Password: "s3cret pass"}); err != nil {
		t.Fatal(err)
	}
	for _, call := range run.calls {
		if strings.Contains(call, "s3cret") {
			t.Errorf("the password is on a command line: %q", call)
		}
	}
	path := filepath.Join(dir, "ohmyserver-"+uuid+".nmconnection")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v, %v", info, err)
	}
	text, _ := os.ReadFile(path)
	for _, want := range []string{"uuid=" + uuid, "ssid=NewNet", "key-mgmt=wpa-psk", "psk=s3cret pass", "method=auto"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	got := changes(run.calls)
	want := []string{"nmcli connection load " + path, "nmcli --wait 30 connection up " + uuid + " ifname wlan0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
}

// A saved network is joined as it is saved: nothing is written.
func TestConnectWifiToASavedNetworkWritesNothing(t *testing.T) {
	dir := t.TempDir()
	run := wifiRunner()
	if err := (&NetworkManager{Run: run, ConnDir: dir}).ConnectWifi(context.Background(), "wlan0", WifiRequest{SSID: "HomeNet", Security: "wpa-psk"}); err != nil {
		t.Fatal(err)
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Errorf("wrote %v", files)
	}
	if got := changes(run.calls); !reflect.DeepEqual(got, []string{"nmcli --wait 30 connection up u-home ifname wlan0"}) {
		t.Errorf("calls %q", got)
	}
}

func TestConnectWifiCleansUpWhatItCouldNotLoad(t *testing.T) {
	dir := t.TempDir()
	uuid := wifiUUID("NewNet")
	run := wifiRunner()
	run.fail = map[string]bool{"nmcli connection load " + filepath.Join(dir, "ohmyserver-"+uuid+".nmconnection"): true}
	err := (&NetworkManager{Run: run, ConnDir: dir}).ConnectWifi(context.Background(), "wlan0", WifiRequest{SSID: "NewNet", Security: "wpa-psk", Password: "password1"})
	if err == nil {
		t.Fatal("no error")
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Errorf("left %v behind, holding a password", files)
	}
}

// A name NetworkManager reads differently from how it was written would
// join the wrong network, so it is deleted and refused.
func TestConnectWifiRefusesAMisreadName(t *testing.T) {
	dir := t.TempDir()
	uuid := wifiUUID("1;2;3")
	run := wifiRunner()
	run.answers[readbackKey(uuid)] = "\x01\x02\x03\n"
	err := (&NetworkManager{Run: run, ConnDir: dir}).ConnectWifi(context.Background(), "wlan0", WifiRequest{SSID: "1;2;3", Security: "open"})
	if err == nil || !strings.Contains(err.Error(), "did not read") {
		t.Fatalf("err = %v", err)
	}
	if indexOf(changes(run.calls), "connection delete "+uuid) < 0 {
		t.Errorf("not deleted: %q", run.calls)
	}
	if indexOf(run.calls, "connection up") >= 0 {
		t.Errorf("went on to join: %q", run.calls)
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Errorf("left %v behind", files)
	}
}

func TestWifiKeyfileEscapesAndOmitsWhatIsNotNeeded(t *testing.T) {
	open := wifiKeyfile("u", WifiRequest{SSID: " pad\\ ", Security: "open", Hidden: true})
	if !strings.Contains(open, `ssid=\spad\\\s`) || !strings.Contains(open, "hidden=true") || strings.Contains(open, "wifi-security") {
		t.Errorf("open:\n%s", open)
	}
	if secured := wifiKeyfile("u", WifiRequest{SSID: "x", Security: "sae", Password: "password1"}); !strings.Contains(secured, "key-mgmt=sae") || strings.Contains(secured, "hidden") {
		t.Errorf("secured:\n%s", secured)
	}
}

func TestWifiUUIDIsStableAndPerName(t *testing.T) {
	a, b := wifiUUID("Home"), wifiUUID("Home")
	if a != b || a == wifiUUID("Home ") || len(a) != 36 {
		t.Errorf("uuids %q %q", a, b)
	}
}
