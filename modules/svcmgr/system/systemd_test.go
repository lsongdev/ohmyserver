package system

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	outputs map[string]string
	ran     []string
}

func (f *fakeRunner) Output(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.ran = append(f.ran, line)
	return []byte(f.outputs[line]), nil
}

func (f *fakeRunner) Stream(ctx context.Context, env []string, out func(string), name string, args ...string) error {
	data, err := f.Output(ctx, env, name, args...)
	for _, line := range lines(data) {
		out(line)
	}
	return err
}

// Recorded on Alibaba Cloud Linux 3 (systemd 239).
const listUnits = `display-manager.service                               not-found inactive dead    display-manager.service
aegis.service                                         loaded    active   running Aegis Service                                                     
auditd.service                                        loaded    inactive dead    Security Auditing Service                                         
● nginx.service                                       loaded    failed   failed  The nginx HTTP and reverse proxy server
systemd-fsck@dev-disk-by\x2duuid-1234.service         loaded    active   exited  File System Check on /dev/disk/by-uuid/1234
`

const unitFiles = `aegis.service                              enabled        
arp-ethers.service                         disabled       
auditd.service                             enabled        
getty@.service                             enabled        
dbus-org.freedesktop.login1.service        alias          
nginx.service                              disabled       
`

func TestServices(t *testing.T) {
	run := &fakeRunner{outputs: map[string]string{
		"systemctl list-units --type=service --all --no-legend --no-pager --plain": listUnits,
		"systemctl list-unit-files --type=service --no-legend --no-pager":          unitFiles,
	}}
	units, err := (&Systemd{Run: run}).Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range units {
		got = append(got, u.Name+"|"+u.Active+"|"+u.Sub+"|"+u.Enabled+"|"+u.Description)
	}
	want := []string{
		"aegis.service|active|running|enabled|Aegis Service",
		"arp-ethers.service|inactive|dead|disabled|",
		"auditd.service|inactive|dead|enabled|Security Auditing Service",
		"nginx.service|failed|failed|disabled|The nginx HTTP and reverse proxy server",
		`systemd-fsck@dev-disk-by\x2duuid-1234.service|active|exited||File System Check on /dev/disk/by-uuid/1234`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
}

func TestParseShow(t *testing.T) {
	out := `MainPID=791245
Result=success
NRestarts=0
ExecMainStatus=0
MemoryCurrent=18849792
CPUUsageNSec=[not set]
TasksCurrent=9
Id=derper.service
Description=Tailscale DERP relay (derp.sczh.cc)
LoadState=loaded
ActiveState=active
SubState=running
FragmentPath=/etc/systemd/system/derper.service
UnitFileState=enabled
UnitFilePreset=disabled
ActiveEnterTimestamp=Thu 2026-10-22 13:27:19 CST
InactiveEnterTimestamp=
`
	d := parseShow([]byte(out))
	if d.Name != "derper.service" || d.MainPID != 791245 || d.Memory != 18849792 || d.CPU != 0 || d.Tasks != 9 ||
		d.Enabled != "enabled" || d.Path != "/etc/systemd/system/derper.service" || d.Since == nil {
		t.Fatalf("got %+v", d)
	}
	if got := d.Since.Format("2006-01-02 15:04:05"); got != "2026-10-22 13:27:19" {
		t.Fatalf("since %s", got)
	}
	if d := parseShow([]byte("MemoryCurrent=18446744073709551615\nActiveState=inactive\nInactiveEnterTimestamp=n/a\n")); d.Memory != 0 || d.Since != nil {
		t.Fatalf("unset values: %+v", d)
	}
}

func TestUnitName(t *testing.T) {
	for in, want := range map[string]string{"nginx": "nginx.service", "derper.service": "derper.service", `systemd-fsck@dev-x\x2d1.service`: `systemd-fsck@dev-x\x2d1.service`} {
		if got, err := UnitName(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"-x", "--now", "a b", "a;b", "getty@.service", "../x", "x.socket.service/.."} {
		if _, err := UnitName(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestActRejectsUnknownAction(t *testing.T) {
	run := &fakeRunner{}
	if err := (&Systemd{Run: run}).Act(context.Background(), "nginx.service", "mask", false); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := (&Systemd{Run: run}).Act(context.Background(), "systemd-reboot.service", "start", false); !errors.Is(err, ErrGuarded) {
		t.Fatalf("reboot: %v", err)
	}
	(&Systemd{Run: run}).Act(context.Background(), "ohmyserver.service", "restart", true)
	if len(run.ran) != 1 || run.ran[0] != "systemctl restart --no-block ohmyserver.service" {
		t.Fatalf("ran %q", run.ran)
	}
}

func TestSelfUnit(t *testing.T) {
	v1 := "13:rdma:/\n11:cpu,cpuacct:/system.slice/ohmyserver.service\n1:name=systemd:/system.slice/ohmyserver.service\n"
	v2 := "0::/system.slice/ohmyserver.service\n"
	shell := "1:name=systemd:/user.slice/user-0.slice/session-3.scope\n"
	if parseSelfUnit(v1) != "ohmyserver.service" || parseSelfUnit(v2) != "ohmyserver.service" || parseSelfUnit(shell) != "" {
		t.Fatal(parseSelfUnit(v1), parseSelfUnit(v2), parseSelfUnit(shell))
	}
}

func TestJournal(t *testing.T) {
	entry := func(cursor, msg string, prio int) string {
		return `{ "__CURSOR" : "` + cursor + `", "__REALTIME_TIMESTAMP" : "1793073823314313", "PRIORITY" : "` + string(rune('0'+prio)) +
			`", "SYSLOG_IDENTIFIER" : "derper", "_PID" : "791245", "_SYSTEMD_UNIT" : "derper.service", "MESSAGE" : ` + msg + ` }`
	}
	out := entry("s=1", `"started"`, 6) + "\n" + entry("s=2", `"write: broken pipe"`, 3) + "\n" +
		entry("s=3", `[104,105,27,91,48,109]`, 6) + "\n"
	run := &fakeRunner{outputs: map[string]string{
		"journalctl --no-pager -o json -u derper.service -n 200":   out,
		"journalctl --no-pager -o json -u derper.service -n 20000": out,
	}}
	sd := &Systemd{Run: run}
	got, err := sd.Journal(context.Background(), Query{Unit: "derper"})
	if err != nil || len(got) != 3 {
		t.Fatalf("got %+v %v", got, err)
	}
	if got[1].Priority != 3 || got[1].PID != 791245 || got[1].Unit != "derper.service" || got[2].Message != "hi\x1b[0m" ||
		!got[0].Time.Equal(time.UnixMicro(1793073823314313)) {
		t.Fatalf("entries %+v", got)
	}
	got, _ = sd.Journal(context.Background(), Query{Unit: "derper", Search: "BROKEN", Lines: 10})
	if len(got) != 1 || got[0].Cursor != "s=2" {
		t.Fatalf("search %+v", got)
	}
	for _, q := range []Query{{Unit: "-x"}, {Priority: 9}, {After: "a b"}, {Since: -time.Hour}} {
		if _, err := sd.Journal(context.Background(), q); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v accepted", q)
		}
	}
}

func TestFollowArgs(t *testing.T) {
	q := Query{Unit: "nginx", Priority: 4, After: "s=1;i=2"}
	args, err := q.args(true)
	if err != nil || strings.Join(args, " ") != "--no-pager -o json -u nginx.service -p 4 --after-cursor s=1;i=2 -f -n 0" {
		t.Fatalf("%q %v", args, err)
	}
}
