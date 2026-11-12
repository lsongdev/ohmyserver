package system

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPPPoENormalizeFillsDefaultsAndRefusesWhatCouldReachTheFile(t *testing.T) {
	r, err := PPPoERequest{Name: "Provider", Parent: "eth1", Username: "user@isp", Password: "pw"}.Normalize()
	if err != nil || r.MTU != 1492 || r.Metric != 50 {
		t.Fatalf("%+v, %v", r, err)
	}
	bad := map[string]struct {
		r     PPPoERequest
		field string
	}{
		"empty name":    {PPPoERequest{Parent: "eth1", Username: "u"}, "name"},
		"newline name":  {PPPoERequest{Name: "a\nb", Parent: "eth1", Username: "u"}, "name"},
		"bad parent":    {PPPoERequest{Name: "p", Parent: "eth1;x", Username: "u"}, "parent"},
		"no user":       {PPPoERequest{Name: "p", Parent: "eth1"}, "username"},
		"user newline":  {PPPoERequest{Name: "p", Parent: "eth1", Username: "u\nv"}, "username"},
		"password nl":   {PPPoERequest{Name: "p", Parent: "eth1", Username: "u", Password: "a\nb"}, "password"},
		"service":       {PPPoERequest{Name: "p", Parent: "eth1", Username: "u", Service: "s\n"}, "service"},
		"mtu too small": {PPPoERequest{Name: "p", Parent: "eth1", Username: "u", MTU: 100}, "mtu"},
		"mtu too big":   {PPPoERequest{Name: "p", Parent: "eth1", Username: "u", MTU: 9000}, "mtu"},
		"metric":        {PPPoERequest{Name: "p", Parent: "eth1", Username: "u", Metric: 70000}, "metric"},
	}
	for name, c := range bad {
		_, err := c.r.Normalize()
		if v, ok := err.(*ValidationError); !ok || v.Fields[c.field] == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestPPPoEInterfaceNameFitsLinuxAndDiffersPerConnection(t *testing.T) {
	a, b := pppoeIfname(pppoeUUID("Provider")), pppoeIfname(pppoeUUID("Spare"))
	if len(a) > 15 || !strings.HasPrefix(a, "ppp") || a == b || a != pppoeIfname(pppoeUUID("Provider")) {
		t.Errorf("names %q, %q", a, b)
	}
	if !ValidName(a) {
		t.Errorf("%q is not a valid interface name", a)
	}
}

func TestPPPoEKeyfile(t *testing.T) {
	uuid := pppoeUUID("Provider")
	text := pppoeKeyfile(uuid, PPPoERequest{Name: "Provider", Parent: "eth1", Username: "user@isp", Password: " pa\\ss ", Service: "svc", MTU: 1492, Metric: 50})
	for _, want := range []string{"id=Provider", "uuid=" + uuid, "type=pppoe", "interface-name=" + pppoeIfname(uuid), "parent=eth1", "username=user@isp", `password=\spa\\ss\s`, "service=svc", "mtu=1492", "route-metric=50", "[ipv6]\nmethod=ignore"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(pppoeKeyfile(uuid, PPPoERequest{Name: "p", Parent: "eth1", Username: "u"}), "service=") {
		t.Error("an empty service name was written")
	}
}

// What is written can be read back, spaces and backslashes included.
func TestSavedPasswordRoundTrip(t *testing.T) {
	dir := t.TempDir()
	for _, pw := range []string{"plain", " lead", "trail ", `back\slash`, `\s`, ""} {
		path := filepath.Join(dir, "c")
		os.WriteFile(path, []byte(pppoeKeyfile("u", PPPoERequest{Name: "p", Parent: "eth1", Username: "u", Password: pw})), 0o600)
		if got, ok := savedPassword(path); !ok || got != pw {
			t.Errorf("%q came back as %q (%v)", pw, got, ok)
		}
	}
	if _, ok := savedPassword(filepath.Join(dir, "none")); ok {
		t.Error("a missing file has a password")
	}
}

const listKey = "nmcli -t -f UUID,TYPE connection show"

func pppoeShow(uuid string) string {
	return "nmcli -t -e no -f " + pppoeFields + " connection show " + uuid
}

// pppoeRunner has a connection this module wrote (Provider, up on ppp0) and
// one made some other way (Manual).
func pppoeRunner(dir string) *fakeRunner {
	managed := pppoeUUID("Provider")
	os.WriteFile(filepath.Join(dir, "ohmyserver-"+managed+".nmconnection"),
		[]byte(pppoeKeyfile(managed, PPPoERequest{Name: "Provider", Parent: "eth1", Username: "user@isp", Password: "secret", MTU: 1492, Metric: 50})), 0o600)
	return &fakeRunner{answers: map[string]string{
		listKey: managed + ":pppoe\nu-man:pppoe\nu-eth:802-3-ethernet\n",
		"nmcli -t -f UUID,DEVICE connection show --active": managed + ":ppp0\nu-eth:eth0\n",
		pppoeShow(managed): "connection.id:Provider\nconnection.autoconnect:yes\npppoe.parent:eth1\npppoe.username:user@isp\npppoe.service:--\nppp.mtu:1492\nipv4.route-metric:50\n",
		pppoeShow("u-man"): "connection.id:Manual\nconnection.autoconnect:no\npppoe.parent:eth2\npppoe.username:x\npppoe.service:--\nppp.mtu:--\nipv4.route-metric:-1\n",
	}}
}

func TestPPPoEListMarksWhatIsManagedAndActive(t *testing.T) {
	dir := t.TempDir()
	list, err := (&NetworkManager{Run: pppoeRunner(dir), ConnDir: dir}).PPPoEList(context.Background())
	if err != nil || len(list) != 2 {
		t.Fatalf("%+v, %v", list, err)
	}
	m, o := list[1], list[0] // by name: Manual, Provider
	if o.Name != "Manual" || m.Name != "Provider" {
		t.Fatalf("order: %s, %s", o.Name, m.Name)
	}
	if !m.Managed || !m.Active || m.Device != "ppp0" || !m.HasPassword || m.Parent != "eth1" || m.MTU != 1492 || m.Metric != 50 || m.Service != "" {
		t.Errorf("Provider = %+v", m)
	}
	if o.Managed || o.Active || o.HasPassword || o.Metric != 0 {
		t.Errorf("Manual = %+v", o)
	}
}

func TestPreparePPPoERefusals(t *testing.T) {
	dir := t.TempDir()
	n := &NetworkManager{Run: pppoeRunner(dir), ConnDir: dir}
	_, err := n.PreparePPPoE(context.Background(), PPPoERequest{Name: "Manual", Parent: "eth1", Username: "u", Password: "pw", MTU: 1492, Metric: 50})
	if v, ok := err.(*ValidationError); !ok || v.Fields["name"] == "" {
		t.Errorf("taking over a connection made another way: %v", err)
	}
	_, err = n.PreparePPPoE(context.Background(), PPPoERequest{Name: "Fresh", Parent: "eth1", Username: "u", MTU: 1492, Metric: 50})
	if v, ok := err.(*ValidationError); !ok || v.Fields["password"] == "" {
		t.Errorf("a new connection without a password: %v", err)
	}
}

// A new connection is undone by deleting it and its file. Nothing is copied.
func TestPrepareNewPPPoERecordsItAsCreated(t *testing.T) {
	dir := t.TempDir()
	undo, err := (&NetworkManager{Run: pppoeRunner(dir), ConnDir: dir}).PreparePPPoE(context.Background(),
		PPPoERequest{Name: "Fresh", Parent: "eth1", Username: "u", Password: "pw", MTU: 1492, Metric: 50})
	uuid := pppoeUUID("Fresh")
	if err != nil || len(undo.Profiles) != 1 || !undo.Profiles[0].Created || undo.Profiles[0].UUID != uuid || len(undo.Cleanup) != 0 {
		t.Fatalf("%+v, %v", undo, err)
	}
	if undo.Profiles[0].File != filepath.Join(dir, "ohmyserver-"+uuid+".nmconnection") {
		t.Errorf("file = %q", undo.Profiles[0].File)
	}
}

// An edit keeps the password when none is given, copies the file first, and
// the copy is for the revert alone: it is hidden, private, and removed when
// the change is kept.
func TestEditingPPPoEKeepsThePasswordAndABackupForTheRevert(t *testing.T) {
	dir := t.TempDir()
	run := pppoeRunner(dir)
	n := &NetworkManager{Run: run, ConnDir: dir}
	req := PPPoERequest{Name: "Provider", Parent: "eth1", Username: "new@isp", MTU: 1400, Metric: 50}
	undo, err := n.PreparePPPoE(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	uuid := pppoeUUID("Provider")
	p := undo.Profiles[0]
	if p.Created || p.Backup != filepath.Join(dir, ".ohmyserver-"+uuid+".prev") || !p.Reup || !reflect.DeepEqual(undo.Cleanup, []string{p.Backup}) {
		t.Fatalf("undo = %+v", undo)
	}
	if info, err := os.Stat(p.Backup); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("backup: %v, %v", info, err)
	}
	if err := n.SavePPPoE(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(p.File)
	for _, want := range []string{"username=new@isp", "password=secret", "mtu=1400"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	// The old file is what the backup holds.
	if old, _ := os.ReadFile(p.Backup); !strings.Contains(string(old), "username=user@isp") {
		t.Errorf("backup = %s", old)
	}
	for _, call := range run.calls {
		if strings.Contains(call, "secret") {
			t.Errorf("the password is on a command line: %q", call)
		}
	}
	want := []string{"nmcli connection load " + p.File, "nmcli connection down " + uuid, "nmcli --wait 45 connection up " + uuid}
	if got := changes(run.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
}

func TestRevertingAnEditPutsTheOldFileBackAndRedials(t *testing.T) {
	dir := t.TempDir()
	run := pppoeRunner(dir)
	n := &NetworkManager{Run: run, ConnDir: dir}
	req := PPPoERequest{Name: "Provider", Parent: "eth1", Username: "new@isp", MTU: 1400, Metric: 50}
	undo, _ := n.PreparePPPoE(context.Background(), req)
	n.SavePPPoE(context.Background(), req)
	run.calls = nil
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	p := undo.Profiles[0]
	if text, _ := os.ReadFile(p.File); !strings.Contains(string(text), "username=user@isp") {
		t.Errorf("file = %s", text)
	}
	if _, err := os.Stat(p.Backup); err == nil {
		t.Error("backup still there")
	}
	want := []string{"nmcli connection load " + p.File, "nmcli connection down " + pppoeUUID("Provider"), "nmcli --wait 45 connection up " + pppoeUUID("Provider")}
	if got := changes(run.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
	// A revert that is retried finds it done already.
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Errorf("second revert: %v", err)
	}
}

func TestRevertingANewConnectionDeletesItAndItsFile(t *testing.T) {
	dir := t.TempDir()
	run := pppoeRunner(dir)
	n := &NetworkManager{Run: run, ConnDir: dir}
	req := PPPoERequest{Name: "Fresh", Parent: "eth1", Username: "u", Password: "pw", MTU: 1492, Metric: 50}
	undo, _ := n.PreparePPPoE(context.Background(), req)
	n.SavePPPoE(context.Background(), req)
	file := undo.Profiles[0].File
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("not written: %v", err)
	}
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("the file, with its password, was left behind")
	}
	if indexOf(run.calls, "connection delete "+pppoeUUID("Fresh")) < 0 {
		t.Errorf("not deleted: %q", run.calls)
	}
}

func TestDialHangupAndForget(t *testing.T) {
	dir := t.TempDir()
	run := pppoeRunner(dir)
	n := &NetworkManager{Run: run, ConnDir: dir}
	uuid := pppoeUUID("Provider")
	// Provider is up, so a dial has nothing to hang up afterwards. Manual is
	// down, so the revert of dialing it is a hang-up.
	if undo, err := n.PrepareDial(context.Background(), "Provider"); err != nil || len(undo.Profiles) != 0 {
		t.Errorf("active: %+v, %v", undo, err)
	}
	undo, err := n.PrepareDial(context.Background(), "Manual")
	if err != nil || len(undo.Profiles) != 1 || !undo.Profiles[0].Down || undo.Profiles[0].UUID != "u-man" {
		t.Fatalf("inactive: %+v, %v", undo, err)
	}
	if err := n.Dial(context.Background(), "Manual"); err != nil {
		t.Fatal(err)
	}
	run.calls = nil
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if got := changes(run.calls); !reflect.DeepEqual(got, []string{"nmcli connection down u-man"}) {
		t.Errorf("revert: %q", got)
	}
	run.calls = nil
	if err := n.Hangup(context.Background(), "Provider"); err != nil || !reflect.DeepEqual(changes(run.calls), []string{"nmcli connection down " + uuid}) {
		t.Errorf("hangup: %q, %v", run.calls, err)
	}
	backup := n.profileBackup(uuid)
	os.WriteFile(backup, []byte("old password"), 0o600)
	if err := n.ForgetPPPoE(context.Background(), "Provider"); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{n.profilePath(uuid), backup} {
		if _, err := os.Stat(file); err == nil {
			t.Errorf("%s survived forgetting", file)
		}
	}
	if err := n.Hangup(context.Background(), "Nope"); err != ErrNotFound {
		t.Errorf("unknown: %v", err)
	}
}
