package system

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const uplinkFile = `# LAN uplink
[Match]
MACAddress=e4:3a:6e:73:14:33

[Network]
Address=192.168.8.100/24
Gateway=192.168.8.1
DNS=192.168.8.1
IPv6AcceptRA=yes
DHCP=ipv6
`

func TestReadNetworkFile(t *testing.T) {
	c, preserved := configOf(parseUnit(uplinkFile))
	if c.DHCP4 || !c.DHCP6 {
		t.Errorf("dhcp = %v, %v", c.DHCP4, c.DHCP6)
	}
	if !reflect.DeepEqual(c.Addresses, []string{"192.168.8.100/24"}) || c.Gateway4 != "192.168.8.1" || !reflect.DeepEqual(c.DNS, []string{"192.168.8.1"}) {
		t.Errorf("config = %+v", c)
	}
	if !reflect.DeepEqual(preserved, []string{"[Network] IPv6AcceptRA=yes"}) {
		t.Errorf("preserved = %q", preserved)
	}
}

func TestUnchangedConfigKeepsFile(t *testing.T) {
	u := parseUnit(uplinkFile)
	c, _ := configOf(u)
	applyConfig(u, c)
	if got := u.String(); got != uplinkFile {
		t.Errorf("rewrote the file:\n%s", got)
	}
}

func TestChangeOnlyTouchesOwnedLines(t *testing.T) {
	u := parseUnit(uplinkFile + "\n[Route]\nDestination=10.0.0.0/8\nGateway=192.168.8.2\nMetric=50\n")
	c, preserved := configOf(u)
	if len(preserved) != 2 {
		t.Fatalf("preserved = %q", preserved)
	}
	c.Addresses = []string{"192.168.8.101/24", "fd00::1/64"}
	c.DHCP4, c.DHCP6 = true, true
	c.MTU = 9000
	c.Routes = []StaticRoute{{Destination: "10.1.0.0/16", Gateway: "192.168.8.3"}}
	applyConfig(u, c)
	got := u.String()
	for _, want := range []string{
		"# LAN uplink\n", "MACAddress=e4:3a:6e:73:14:33\n", "IPv6AcceptRA=yes\n", "DHCP=yes\n",
		"Address=192.168.8.101/24\nAddress=fd00::1/64\nGateway=192.168.8.1\n",
		"[Link]\nMTUBytes=9000\n",
		"Metric=50\n", // a route the form cannot show stays
		"[Route]\nDestination=10.1.0.0/16\nGateway=192.168.8.3\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "DHCP=ipv6") || strings.Contains(got, "192.168.8.100") {
		t.Errorf("old values remain:\n%s", got)
	}
	// Parsing it again sees what was written.
	again, _ := configOf(parseUnit(got))
	if !reflect.DeepEqual(again.Routes, c.Routes) || !reflect.DeepEqual(again.Addresses, c.Addresses) || again.MTU != 9000 {
		t.Errorf("read back %+v", again)
	}
}

func TestRemovingDHCPSaysNoOnlyIfItWasThere(t *testing.T) {
	u := parseUnit("[Match]\nName=eth0\n\n[Network]\nDHCP=yes\n")
	c, _ := configOf(u)
	c.DHCP4, c.DHCP6 = false, false
	applyConfig(u, c)
	if !strings.Contains(u.String(), "DHCP=no\n") {
		t.Errorf("got:\n%s", u)
	}
	u = parseUnit("[Match]\nName=eth0\n")
	applyConfig(u, Config{Addresses: []string{"10.0.0.2/24"}})
	if strings.Contains(u.String(), "DHCP") {
		t.Errorf("got:\n%s", u)
	}
}

func TestSpecialGatewayRouteIsPreserved(t *testing.T) {
	u := parseUnit("[Match]\nName=eth0\n\n[Route]\nGateway=_dhcp4\nDestination=10.0.0.0/8\n")
	c, preserved := configOf(u)
	if len(c.Routes) != 0 || len(preserved) != 1 {
		t.Fatalf("routes %v, preserved %q", c.Routes, preserved)
	}
	applyConfig(u, c)
	if !strings.Contains(u.String(), "Gateway=_dhcp4\n") {
		t.Errorf("lost it:\n%s", u)
	}
}

func TestMatchReason(t *testing.T) {
	for text, editable := range map[string]bool{
		"[Match]\nName=eth0\n":                    true,
		"[Match]\nMACAddress=aa:bb:cc:dd:ee:ff\n": true,
		"[Match]\nName=en*\n":                     false,
		"[Match]\nName=eth0 eth1\n":               false,
		"[Match]\nType=ether\n":                   false,
		"[Network]\nDHCP=yes\n":                   false,
	} {
		if got := matchReason(parseUnit(text)) == ""; got != editable {
			t.Errorf("%q: editable = %v", text, got)
		}
	}
}

func TestNormalize(t *testing.T) {
	good, err := Config{
		Addresses: []string{" 192.168.1.10/24 ", "FD00::1/64"}, Gateway4: "192.168.1.1", Gateway6: "fe80::1",
		DNS: []string{"1.1.1.1", "", "9.9.9.9:853#dns.quad9.net"}, MTU: 1500,
		Routes: []StaticRoute{{Destination: "10.1.2.3/16", Gateway: "192.168.1.254"}, {Destination: "fd01::/48"}},
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if good.Addresses[1] != "fd00::1/64" || good.Routes[0].Destination != "10.1.0.0/16" || len(good.DNS) != 2 {
		t.Errorf("normalized = %+v", good)
	}
	for field, bad := range map[string]Config{
		"addresses": {Addresses: []string{"192.168.1.10"}},
		"gateway4":  {Addresses: []string{"192.168.1.10/24"}, Gateway4: "10.0.0.1"},
		"gateway6":  {Gateway6: "10.0.0.1"},
		"dns":       {DNS: []string{"1.1.1.1\nAddress=6.6.6.6/32"}},
		"mtu":       {MTU: 12},
		"routes":    {Routes: []StaticRoute{{Destination: "10.0.0.0/8", Gateway: "fd00::1"}}},
	} {
		_, err := bad.Normalize()
		v, ok := err.(*ValidationError)
		if !ok || v.Fields[field] == "" {
			t.Errorf("%s: err = %v", field, err)
		}
	}
	// A network's own address is a typo, and would not work.
	if _, err := (Config{Addresses: []string{"192.168.1.0/24"}}).Normalize(); err == nil {
		t.Error("accepted a network address")
	}
	// With DHCP the gateway may lie in the subnet DHCP brings.
	if _, err := (Config{DHCP4: true, Gateway4: "10.0.0.1"}).Normalize(); err != nil {
		t.Error(err)
	}
}

func TestParseLinks(t *testing.T) {
	links, err := parseLinks([]byte(`[
	 {"ifname":"lo","flags":["LOOPBACK","UP","LOWER_UP"],"mtu":65536,"operstate":"UNKNOWN","link_type":"loopback","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8,"scope":"host","valid_life_time":4294967295}]},
	 {"ifname":"enp5s0","flags":["BROADCAST","UP","LOWER_UP"],"mtu":1500,"operstate":"UP","link_type":"ether","address":"e4:3a:6e:73:14:33",
	  "addr_info":[{"family":"inet6","local":"fd97::79a","prefixlen":128,"scope":"global","dynamic":true,"valid_life_time":5390}],
	  "stats64":{"rx":{"bytes":100,"packets":2,"errors":1,"dropped":3},"tx":{"bytes":200,"packets":4,"errors":0,"dropped":0}}},
	 {"ifname":"docker0","flags":["UP"],"mtu":1500,"operstate":"DOWN","link_type":"ether","linkinfo":{"info_kind":"bridge"}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{links[0].Name, links[1].Name, links[2].Name}
	if !reflect.DeepEqual(names, []string{"docker0", "enp5s0", "lo"}) {
		t.Errorf("order = %v", names)
	}
	e := links[1]
	if e.Kind != "ethernet" || !e.Up || !e.Carrier || e.RXBytes != 100 || e.TXBytes != 200 || e.RXDropped != 3 {
		t.Errorf("link = %+v", e)
	}
	if a := e.Addresses[0]; !a.Dynamic || a.ValidLife != 5390 {
		t.Errorf("address = %+v", a)
	}
	if links[2].Addresses[0].ValidLife != 0 {
		t.Errorf("a permanent address has no lifetime")
	}
	if links[0].Kind != "bridge" || EditableReason(links[0]) == "" || EditableReason(links[2]) == "" || EditableReason(e) != "" {
		t.Errorf("editable: %+v", links)
	}
}

func TestParsePorts(t *testing.T) {
	ports := parsePorts(`udp UNCONN 0 0 127.0.0.53%lo:53 0.0.0.0:* users:(("systemd-resolve",pid=500,fd=14))
tcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:(("sshd",pid=1,fd=3),("sshd",pid=2,fd=3))
tcp LISTEN 0 128 [::]:22 [::]:*
garbage
`)
	if len(ports) != 3 {
		t.Fatalf("ports = %+v", ports)
	}
	if ports[0].Address != "0.0.0.0" || ports[0].Port != 22 || ports[0].Process != "sshd" || ports[0].PID != 1 {
		t.Errorf("first = %+v", ports[0])
	}
	if ports[1].Address != "::" || ports[1].Process != "" {
		t.Errorf("second = %+v", ports[1])
	}
	if ports[2].Address != "127.0.0.53%lo" || ports[2].Protocol != "udp" {
		t.Errorf("third = %+v", ports[2])
	}
}

func TestParseResolvers(t *testing.T) {
	servers, search := parseResolvConf("# comment\nnameserver 127.0.0.53\nsearch lan example.com\n")
	if !reflect.DeepEqual(servers, []string{"127.0.0.53"}) || !reflect.DeepEqual(search, []string{"lan", "example.com"}) {
		t.Errorf("%v %v", servers, search)
	}
	global, links := parseResolvectl("Global: 9.9.9.9\nLink 2 (enp4s0):\nLink 3 (enp5s0): 192.168.8.1 fd97::1\n")
	if !reflect.DeepEqual(global, []string{"9.9.9.9"}) || len(links) != 1 || links["enp5s0"][1] != "fd97::1" {
		t.Errorf("%v %v", global, links)
	}
}

// fakeRunner answers commands from a table and records them.
type fakeRunner struct {
	answers map[string]string
	fail    map[string]bool
	calls   []string
}

func (f *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, []byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.fail[call] {
		return nil, nil, &ExitError{Command: name, Code: 1, Output: "failed"}
	}
	return []byte(f.answers[call]), nil, nil
}

func TestNetworkdApplyAndRevert(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "10-uplink.network")
	if err := os.WriteFile(file, []byte(uplinkFile), 0o644); err != nil {
		t.Fatal(err)
	}
	run := &fakeRunner{answers: map[string]string{
		"networkctl --json=short status enp5s0": `{"NetworkFile":"` + file + `","NetworkFileDropins":[]}`,
		"networkctl --json=short list":          `{"Interfaces":[{"Name":"enp5s0"}]}`,
	}}
	n := &Networkd{Run: run, Dir: dir}
	link := Link{Name: "enp5s0", Kind: "ethernet"}
	s, err := n.Read(context.Background(), link)
	if err != nil || s.Reason != "" || !s.Exists {
		t.Fatalf("read: %+v, %v", s, err)
	}
	undo, err := n.Prepare(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Config
	c.Addresses = []string{"192.168.8.200/24"}
	plan, err := n.Plan(context.Background(), link, c)
	if err != nil || !strings.Contains(plan.After, "192.168.8.200") || !strings.Contains(plan.Before, "192.168.8.100") {
		t.Fatalf("plan: %+v, %v", plan, err)
	}
	if data, _ := os.ReadFile(file); string(data) != uplinkFile {
		t.Fatal("planning wrote the file")
	}
	if err := n.Apply(context.Background(), link, c); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(file); !strings.Contains(string(data), "192.168.8.200/24") {
		t.Errorf("applied:\n%s", data)
	}
	if data, _ := os.ReadFile(file + ".bak"); string(data) != uplinkFile {
		t.Errorf("backup:\n%s", data)
	}
	if got := run.calls[len(run.calls)-1]; got != "networkctl reconfigure enp5s0" {
		t.Errorf("last command %q", got)
	}
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(file); string(data) != uplinkFile {
		t.Errorf("reverted:\n%s", data)
	}
}

func TestNetworkdCreatesFileForUnmatchedLinkAndRemovesIt(t *testing.T) {
	dir := t.TempDir()
	run := &fakeRunner{answers: map[string]string{"networkctl --json=short status eth1": `{"NetworkFile":""}`}}
	n := &Networkd{Run: run, Dir: dir}
	link := Link{Name: "eth1", Kind: "ethernet"}
	undo, err := n.Prepare(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Apply(context.Background(), link, Config{Addresses: []string{"10.0.0.2/24"}}); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(dir, "10-ohmyserver-eth1.network")
	data, err := os.ReadFile(created)
	if err != nil || !strings.Contains(string(data), "[Match]\nName=eth1\n") || !strings.Contains(string(data), "Address=10.0.0.2/24\n") {
		t.Fatalf("%s, %v", data, err)
	}
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("file remains: %v", err)
	}
}

func TestNetworkdRefusesFilesItCannotEditSafely(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "20-all.network")
	os.WriteFile(shared, []byte("[Match]\nName=eth0\n"), 0o644)
	glob := filepath.Join(dir, "30-glob.network")
	os.WriteFile(glob, []byte("[Match]\nName=en*\n"), 0o644)
	for name, answers := range map[string]map[string]string{
		"generated": {"networkctl --json=short status eth0": `{"NetworkFile":"/run/systemd/network/10-netplan-eth0.network"}`},
		"dropins":   {"networkctl --json=short status eth0": `{"NetworkFile":"` + shared + `","NetworkFileDropins":["/etc/x.conf"]}`},
		"glob":      {"networkctl --json=short status eth0": `{"NetworkFile":"` + glob + `"}`},
		"shared": {
			"networkctl --json=short status eth0": `{"NetworkFile":"` + shared + `"}`,
			"networkctl --json=short status eth1": `{"NetworkFile":"` + shared + `"}`,
			"networkctl --json=short list":        `{"Interfaces":[{"Name":"eth0"},{"Name":"eth1"}]}`,
		},
	} {
		n := &Networkd{Run: &fakeRunner{answers: answers}, Dir: dir}
		s, err := n.Read(context.Background(), Link{Name: "eth0"})
		// The generated file does not exist here; reading it fails, which also refuses.
		if name == "generated" {
			if err == nil && s.Reason == "" {
				t.Errorf("%s: editable", name)
			}
			continue
		}
		if err != nil || s.Reason == "" {
			t.Errorf("%s: %+v, %v", name, s, err)
		}
	}
}

func nmRunner() *fakeRunner {
	return &fakeRunner{answers: map[string]string{
		"nmcli -t -e no -f GENERAL.STATE,GENERAL.CON-UUID device show eth0": "GENERAL.STATE:100 (connected)\nGENERAL.CON-UUID:abc-123\n",
		"nmcli -t -e no -f connection.id,connection.type,ipv4.method,ipv4.addresses,ipv4.gateway,ipv4.dns,ipv4.routes,ipv6.method,ipv6.addresses,ipv6.gateway,ipv6.dns,ipv6.routes,ipv4.route-metric,ipv6.route-metric connection show abc-123": "connection.id:System eth0\nconnection.type:802-3-ethernet\nipv4.method:manual\nipv4.addresses:192.168.1.10/24\nipv4.gateway:192.168.1.1\nipv4.dns:1.1.1.1,8.8.8.8\nipv4.routes:10.0.0.0/8 192.168.1.2\nipv6.method:ignore\nipv6.addresses:\nipv6.gateway:\nipv6.dns:\nipv6.routes:\nipv4.route-metric:200\nipv6.route-metric:-1\n",
		"nmcli -t -e no -f 802-3-ethernet.mtu connection show abc-123": "802-3-ethernet.mtu:1400\n",
	}}
}

func TestNetworkManagerRead(t *testing.T) {
	n := &NetworkManager{Run: nmRunner()}
	s, err := n.Read(context.Background(), Link{Name: "eth0", Kind: "ethernet"})
	if err != nil {
		t.Fatal(err)
	}
	c := s.Config
	if c.DHCP4 || c.DHCP6 || c.Addresses[0] != "192.168.1.10/24" || c.Gateway4 != "192.168.1.1" || len(c.DNS) != 2 || c.MTU != 1400 ||
		c.Routes[0] != (StaticRoute{"10.0.0.0/8", "192.168.1.2"}) || c.Metric != 200 || s.Target != "Connection System eth0" {
		t.Errorf("setting = %+v", s)
	}
}

func TestNetworkManagerApplyKeepsDisabledIPv6AndFallsBackToUp(t *testing.T) {
	run := nmRunner()
	run.fail = map[string]bool{"nmcli device reapply eth0": true}
	n := &NetworkManager{Run: run}
	link := Link{Name: "eth0", Kind: "ethernet"}
	undo, err := n.Prepare(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := Config{DHCP4: true, Addresses: []string{}, DNS: []string{"fd00::53"}}.Normalize()
	if err := n.Apply(context.Background(), link, c); err != nil {
		t.Fatal(err)
	}
	var modify string
	for _, call := range run.calls {
		if strings.HasPrefix(call, "nmcli connection modify") {
			modify = call
		}
	}
	for _, want := range []string{"ipv4.method auto", "ipv6.method ignore", "ipv6.dns fd00::53", "802-3-ethernet.mtu auto", "ipv4.route-metric -1", "ipv6.route-metric -1"} {
		if !strings.Contains(modify, want) {
			t.Errorf("missing %q in %q", want, modify)
		}
	}
	if got := run.calls[len(run.calls)-1]; got != "nmcli connection up abc-123 ifname eth0" {
		t.Errorf("last = %q", got)
	}
	run.calls = nil
	if err := n.Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run.calls[0], "ipv4.addresses 192.168.1.10/24") || !strings.Contains(run.calls[0], "ipv4.method manual") || !strings.Contains(run.calls[0], "802-3-ethernet.mtu 1400") || !strings.Contains(run.calls[0], "ipv4.route-metric 200") || !strings.Contains(run.calls[0], "ipv6.route-metric -1") {
		t.Errorf("revert = %q", run.calls[0])
	}
}

func TestRouteMetricIsWrittenToBothFamilies(t *testing.T) {
	p := wanted(Config{DHCP4: true, Metric: 50}, map[string]string{"connection.type": "802-3-ethernet"})
	if p["ipv4.route-metric"] != "50" || p["ipv6.route-metric"] != "50" {
		t.Errorf("metric = %q, %q", p["ipv4.route-metric"], p["ipv6.route-metric"])
	}
	p = wanted(Config{DHCP4: true}, map[string]string{"connection.type": "802-3-ethernet"})
	if p["ipv4.route-metric"] != "-1" {
		t.Errorf("automatic metric = %q", p["ipv4.route-metric"])
	}
}

func TestRouteMetricReadsEitherFamilyAndTreatsAutomaticAsZero(t *testing.T) {
	for _, c := range []struct {
		v4, v6 string
		want   int
	}{{"-1", "-1", 0}, {"", "", 0}, {"100", "-1", 100}, {"-1", "300", 300}, {"junk", "", 0}} {
		if got := routeMetric(c.v4, c.v6); got != c.want {
			t.Errorf("routeMetric(%q, %q) = %d, want %d", c.v4, c.v6, got, c.want)
		}
	}
}

func TestMetricIsValidated(t *testing.T) {
	for _, m := range []int{-1, 65536} {
		if _, err := (Config{DHCP4: true, Metric: m}).Normalize(); err == nil {
			t.Errorf("metric %d accepted", m)
		}
	}
	c, err := (Config{DHCP4: true, Metric: 600}).Normalize()
	if err != nil || c.Metric != 600 {
		t.Errorf("metric 600: %+v, %v", c, err)
	}
}

func TestOnlyNetworkManagerSetsMetric(t *testing.T) {
	if !SupportsMetric(&NetworkManager{}) || SupportsMetric(&Networkd{}) {
		t.Error("wrong backends support a metric")
	}
}
