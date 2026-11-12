package system

import (
	"context"
	"errors"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
)

// A change that touched several profiles puts them back in the opposite
// order: a created profile is deleted, a modified one is restored and
// reapplied on its device.
func TestNetworkManagerRevertWalksProfilesBackwards(t *testing.T) {
	run := &fakeRunner{}
	n := &NetworkManager{Run: run}
	undo := Undo{Backend: "NetworkManager", Link: "eth1", Profiles: []ProfileUndo{
		{UUID: "parent-1", Device: "eth1", Props: map[string]string{"ipv4.method": "auto"}},
		{Name: "ohmyserver-ppp0", Created: true},
	}}
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"nmcli connection delete ohmyserver-ppp0",
		"nmcli connection modify parent-1 ipv4.method auto",
		"nmcli device reapply eth1",
	}
	if !reflect.DeepEqual(run.calls, want) {
		t.Errorf("calls = %q, want %q", run.calls, want)
	}
}

// A device with no profile gets one that Prepare records as created, so a
// revert deletes it by name.
func TestNetworkManagerPrepareRecordsCreatedProfile(t *testing.T) {
	run := &fakeRunner{answers: map[string]string{
		"nmcli -t -e no -f GENERAL.STATE,GENERAL.CON-UUID device show eth1": "GENERAL.STATE:30 (disconnected)\nGENERAL.CON-UUID:\n",
	}}
	n := &NetworkManager{Run: run}
	undo, err := n.Prepare(context.Background(), Link{Name: "eth1", Kind: "ethernet"})
	if err != nil {
		t.Fatal(err)
	}
	if len(undo.Profiles) != 1 || !undo.Profiles[0].Created || undo.Profiles[0].Name != "ohmyserver-eth1" {
		t.Errorf("profiles = %+v", undo.Profiles)
	}
}

func TestRevertStopsAtTheFirstFailure(t *testing.T) {
	run := &fakeRunner{fail: map[string]bool{"nmcli connection delete ohmyserver-ppp0": true}}
	n := &NetworkManager{Run: run}
	undo := Undo{Link: "eth1", Profiles: []ProfileUndo{
		{UUID: "parent-1", Device: "eth1", Props: map[string]string{"ipv4.method": "auto"}},
		{Name: "ohmyserver-ppp0", Created: true},
	}}
	if err := n.Revert(context.Background(), undo); err == nil {
		t.Fatal("no error")
	}
	if len(run.calls) != 1 {
		t.Errorf("went on after failing: %q", run.calls)
	}
}

func stubHost(t *testing.T, pppd, plugin bool) {
	t.Helper()
	oldLook, oldGlob, oldStat := lookPath, glob, stat
	t.Cleanup(func() { lookPath, glob, stat = oldLook, oldGlob, oldStat })
	lookPath = func(string) (string, error) {
		if pppd {
			return "/usr/sbin/pppd", nil
		}
		return "", errors.New("not found")
	}
	stat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	glob = func(string) ([]string, error) {
		if plugin {
			return []string{"/usr/lib/x/NetworkManager/1/libnm-ppp-plugin.so"}, nil
		}
		return nil, nil
	}
}

func TestDetectFeatures(t *testing.T) {
	run := &fakeRunner{answers: map[string]string{
		"nmcli -t -e no -f DEVICE,TYPE device": "eth0:ethernet\nwlan0:wifi\nwlan1:wifi\nlo:loopback\np2p-dev-wlan0:wifi-p2p\n",
	}}
	n := &NetworkManager{Run: run}

	stubHost(t, false, true)
	f := n.DetectFeatures(context.Background())
	if !reflect.DeepEqual(f.WiFi, []string{"wlan0", "wlan1"}) {
		t.Errorf("wifi = %v", f.WiFi)
	}
	if f.PPPoE.Ready || !reflect.DeepEqual(f.PPPoE.Missing, []string{"pppd"}) {
		t.Errorf("pppoe = %+v", f.PPPoE)
	}

	stubHost(t, false, false)
	if f := n.DetectFeatures(context.Background()); len(f.PPPoE.Missing) != 2 {
		t.Errorf("missing = %v", f.PPPoE.Missing)
	}

	stubHost(t, true, true)
	if f := n.DetectFeatures(context.Background()); !f.PPPoE.Ready || len(f.PPPoE.Missing) != 0 {
		t.Errorf("pppoe = %+v", f.PPPoE)
	}
}

// With no wireless device the list is empty, not null, for the page.
func TestDetectFeaturesWithoutWiFi(t *testing.T) {
	stubHost(t, true, true)
	n := &NetworkManager{Run: &fakeRunner{answers: map[string]string{"nmcli -t -e no -f DEVICE,TYPE device": "eth0:ethernet\n"}}}
	if f := n.DetectFeatures(context.Background()); f.WiFi == nil || len(f.WiFi) != 0 {
		t.Errorf("wifi = %#v", f.WiFi)
	}
}

func probeFor(run Runner, dnsOK bool, reachable string) Probe {
	return Probe{
		Run: run,
		Dial: func(_ context.Context, _, address string) (net.Conn, error) {
			if address != reachable {
				return nil, errors.New("unreachable")
			}
			a, b := net.Pipe()
			b.Close()
			return a, nil
		},
		Lookup: func(context.Context, string) ([]string, error) {
			if !dnsOK {
				return nil, errors.New("no such host")
			}
			return []string{"203.0.113.9"}, nil
		},
		Targets: []string{"1.1.1.1:443", "8.8.8.8:443"},
		Name:    "example.test",
	}
}

func ids(c Connectivity) map[string]bool {
	m := map[string]bool{}
	for _, check := range c.Checks {
		m[check.ID] = check.OK
	}
	return m
}

func TestConnectivityAllPass(t *testing.T) {
	run := &fakeRunner{answers: map[string]string{
		"ip -4 -j route show": `[{"dst":"default","gateway":"10.0.0.1","dev":"eth1","metric":100},{"dst":"default","gateway":"10.0.1.1","dev":"eth0","metric":600}]`,
		"ip -6 -j route show": `[]`,
	}}
	c := probeFor(run, true, "8.8.8.8:443").Check(context.Background())
	if !c.OK() {
		t.Fatalf("checks = %+v", c.Checks)
	}
	// The lowest metric is the route traffic takes.
	if c.Checks[0].Detail != "Default route through eth1" {
		t.Errorf("route = %q", c.Checks[0].Detail)
	}
	if got := run.calls[len(run.calls)-1]; got != "ping -c 1 -W 2 -n 10.0.0.1" {
		t.Errorf("ping = %q", got)
	}
}

// A missing route is reported, and the later steps still run: DNS may work
// through a local resolver while nothing can leave.
func TestConnectivityNoRoute(t *testing.T) {
	run := &fakeRunner{answers: map[string]string{"ip -4 -j route show": `[]`, "ip -6 -j route show": `[]`}}
	c := probeFor(run, true, "").Check(context.Background())
	got := ids(c)
	if c.OK() || got["route"] || got["gateway"] || !got["dns"] || got["internet"] || len(c.Checks) != 4 {
		t.Errorf("checks = %+v", c.Checks)
	}
}

func TestConnectivityGatewaySilentAndDNSBroken(t *testing.T) {
	run := &fakeRunner{
		answers: map[string]string{
			"ip -4 -j route show": `[{"dst":"default","gateway":"10.0.0.1","dev":"eth1"}]`,
			"ip -6 -j route show": `[]`,
		},
		fail: map[string]bool{"ping -c 1 -W 2 -n 10.0.0.1": true},
	}
	got := ids(probeFor(run, false, "1.1.1.1:443").Check(context.Background()))
	if !got["route"] || got["gateway"] || got["dns"] || !got["internet"] {
		t.Errorf("checks = %v", got)
	}
}

// A point-to-point link has no gateway to ask, which is not a failure.
func TestConnectivityPointToPoint(t *testing.T) {
	run := &fakeRunner{answers: map[string]string{
		"ip -4 -j route show": `[{"dst":"default","dev":"ppp0","scope":"link"}]`,
		"ip -6 -j route show": `[]`,
	}}
	c := probeFor(run, true, "1.1.1.1:443").Check(context.Background())
	if !c.OK() || !strings.Contains(c.Checks[1].Detail, "point-to-point") {
		t.Errorf("checks = %+v", c.Checks)
	}
	for _, call := range run.calls {
		if strings.HasPrefix(call, "ping") {
			t.Errorf("pinged: %q", call)
		}
	}
}

// IPv6 link-local gateways need the interface to be reached.
func TestConnectivityPingsIPv6GatewayThroughItsDevice(t *testing.T) {
	run := &fakeRunner{answers: map[string]string{
		"ip -4 -j route show": `[]`,
		"ip -6 -j route show": `[{"dst":"default","gateway":"fe80::1","dev":"eth1"}]`,
	}}
	probeFor(run, true, "1.1.1.1:443").Check(context.Background())
	want := "ping -c 1 -W 2 -n -I eth1 fe80::1"
	for _, call := range run.calls {
		if call == want {
			return
		}
	}
	t.Errorf("calls = %q, want %q", run.calls, want)
}
