package system

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func profKey(uuid string) string {
	return "nmcli -t -e no -f " + profileFields + " connection show " + uuid
}

func propsKey(uuid string) string {
	return "nmcli -t -e no -f connection.id,connection.type," + strings.Join(nmProps, ",") + " connection show " + uuid
}

func profile(id, typ, ifname, master, slave string) string {
	return "connection.id:" + id + "\nconnection.type:" + typ + "\nconnection.interface-name:" + ifname +
		"\nconnection.master:" + master + "\nconnection.slave-type:" + slave + "\n"
}

// bridgeRunner is a machine with the bridge br-lan and two ports: eth1,
// which names the bridge, and eth2, which names its profile's UUID.
func bridgeRunner() *fakeRunner {
	return &fakeRunner{answers: map[string]string{
		"nmcli -t -e no -f UUID connection show": "u-br\nu-p1\nu-p2\n",
		profKey("u-br"):                          profile("br-lan", "bridge", "br-lan", "", ""),
		profKey("u-p1"):                          profile("Port 1", "802-3-ethernet", "eth1", "br-lan", "bridge"),
		profKey("u-p2"):                          profile("Port 2", "802-3-ethernet", "eth2", "u-br", "bridge"),
		"nmcli -t -e no -f bridge.stp connection show u-br": "bridge.stp:yes\n",
		propsKey("u-br"): "connection.id:br-lan\nconnection.type:bridge\nipv4.method:disabled\n",
		propsKey("u-p1"): "connection.id:Port 1\nconnection.type:802-3-ethernet\nipv4.method:manual\nipv4.addresses:10.0.0.2/24\n",
		propsKey("u-p2"): "connection.id:Port 2\nconnection.type:802-3-ethernet\nipv4.method:auto\n",
		"nmcli -t -e no -f GENERAL.STATE,GENERAL.CON-UUID device show eth1": "GENERAL.STATE:100 (connected)\nGENERAL.CON-UUID:u-p1\n",
		"nmcli -t -e no -f GENERAL.STATE,GENERAL.CON-UUID device show eth2": "GENERAL.STATE:100 (connected)\nGENERAL.CON-UUID:u-p2\n",
		"nmcli -t -e no -f GENERAL.STATE,GENERAL.CON-UUID device show eth3": "GENERAL.STATE:100 (connected)\nGENERAL.CON-UUID:u-p3\n",
	}}
}

// changes keeps the commands that change a connection.
func changes(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, "nmcli connection ") || strings.HasPrefix(c, "nmcli device reapply") {
			out = append(out, c)
		}
	}
	return out
}

func indexOf(calls []string, part string) int {
	for i, c := range calls {
		if strings.Contains(c, part) {
			return i
		}
	}
	return -1
}

func TestBridgesListPortsFoundByNameOrByUUID(t *testing.T) {
	n := &NetworkManager{Run: bridgeRunner()}
	bridges, err := n.Bridges(context.Background())
	if err != nil || len(bridges) != 1 {
		t.Fatalf("%+v, %v", bridges, err)
	}
	b := bridges[0]
	if b.Name != "br-lan" || !b.STP || !reflect.DeepEqual(b.Ports, []string{"eth1", "eth2"}) || b.Profile != "br-lan" || b.Version == "" {
		t.Errorf("bridge = %+v", b)
	}
	if none, _ := n.Bridge(context.Background(), "br-other"); none != nil {
		t.Errorf("found %+v", none)
	}
}

// A new bridge is made first, then its ports are given profiles that name
// it, then it comes up, and last its ports.
func TestApplyBridgeMakesBridgeThenPorts(t *testing.T) {
	run := &fakeRunner{
		answers: map[string]string{
			"nmcli -t -e no -f GENERAL.STATE,GENERAL.CON-UUID device show eth1": "GENERAL.STATE:30 (disconnected)\nGENERAL.CON-UUID:\n",
		},
		fail: map[string]bool{"nmcli device reapply br-new": true}, // there is no device yet
	}
	n := &NetworkManager{Run: run}
	if err := n.ApplyBridge(context.Background(), "br-new", BridgeConfig{Ports: []string{"eth1"}}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"nmcli connection add type bridge ifname br-new con-name ohmyserver-br-new bridge.stp no ipv4.method disabled ipv6.method ignore",
		"nmcli connection add type ethernet ifname eth1 con-name ohmyserver-eth1 connection.master br-new connection.slave-type bridge",
		"nmcli device reapply br-new",
		"nmcli connection up ohmyserver-br-new ifname br-new",
		"nmcli connection up ohmyserver-eth1 ifname eth1",
	}
	if got := changes(run.calls); !reflect.DeepEqual(got, want) {
		t.Errorf("calls:\n%q\nwant:\n%q", got, want)
	}
}

// Changing a bridge frees the ports that left and takes the ones that
// came, and leaves the port that stays alone: it would lose its link.
func TestApplyBridgeFreesAndTakesPortsButLeavesTheOnesThatStay(t *testing.T) {
	run := bridgeRunner()
	n := &NetworkManager{Run: run}
	if err := n.ApplyBridge(context.Background(), "br-lan", BridgeConfig{STP: true, Ports: []string{"eth2", "eth3"}}); err != nil {
		t.Fatal(err)
	}
	got := changes(run.calls)
	for _, call := range got {
		if strings.Contains(call, "u-p2") {
			t.Errorf("touched the port that stays: %q", call)
		}
	}
	for _, want := range []string{
		"nmcli connection modify u-br bridge.stp yes",
		"nmcli connection modify u-p1 connection.master  connection.slave-type  ipv4.method auto ipv6.method auto",
		"nmcli connection modify u-p3 connection.master br-lan connection.slave-type bridge",
		"nmcli connection up u-p3 ifname eth3",
		"nmcli connection up u-p1 ifname eth1",
	} {
		if indexOf(got, want) < 0 {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	// The bridge is up before the ports are brought up.
	if indexOf(got, "device reapply br-lan") > indexOf(got, "connection up u-p3") {
		t.Errorf("order: %q", got)
	}
}

// The undo of a new bridge lists the bridge first and each port after, so a
// revert, which walks backwards, frees the ports before it deletes the bridge.
func TestPrepareNewBridgeListsBridgeThenPorts(t *testing.T) {
	run := &fakeRunner{answers: map[string]string{
		"nmcli -t -e no -f UUID connection show":                            "",
		"nmcli -t -e no -f GENERAL.STATE,GENERAL.CON-UUID device show eth1": "GENERAL.STATE:30 (disconnected)\nGENERAL.CON-UUID:\n",
	}}
	undo, err := (&NetworkManager{Run: run}).PrepareBridge(context.Background(), "br-new", BridgeConfig{Ports: []string{"eth1"}})
	if err != nil || len(undo.Profiles) != 2 {
		t.Fatalf("%+v, %v", undo, err)
	}
	if p := undo.Profiles[0]; !p.Created || p.Name != "ohmyserver-br-new" {
		t.Errorf("bridge = %+v", p)
	}
	if p := undo.Profiles[1]; !p.Created || p.Name != "ohmyserver-eth1" || p.Device != "eth1" {
		t.Errorf("port = %+v", p)
	}
}

// An existing bridge is captured with its settings, and so is every port it
// has now and every port it will have, with the master by name.
func TestPrepareExistingBridgeCapturesTheUnionOfPorts(t *testing.T) {
	run := bridgeRunner()
	undo, err := (&NetworkManager{Run: run}).PrepareBridge(context.Background(), "br-lan", BridgeConfig{Ports: []string{"eth2", "eth3"}})
	if err != nil {
		t.Fatal(err)
	}
	var devices []string
	for _, p := range undo.Profiles {
		devices = append(devices, p.Device)
	}
	// eth3 has no profile in the list, but the device names one.
	if !reflect.DeepEqual(devices, []string{"br-lan", "eth1", "eth2", "eth3"}) {
		t.Fatalf("devices = %v", devices)
	}
	b := undo.Profiles[0]
	if b.UUID != "u-br" || b.Props["bridge.stp"] != "yes" {
		t.Errorf("bridge = %+v", b)
	}
	if _, isPort := b.Props["connection.master"]; isPort {
		t.Error("the bridge was captured as a port, so a revert would bring it up like one")
	}
	p1, p2 := undo.Profiles[1], undo.Profiles[2]
	if p1.Props["connection.master"] != "br-lan" || p1.Props["connection.slave-type"] != "bridge" || p1.Props["ipv4.addresses"] != "10.0.0.2/24" {
		t.Errorf("eth1 = %+v", p1.Props)
	}
	// This port named the bridge by UUID; the undo names it by interface.
	if p2.Props["connection.master"] != "br-lan" {
		t.Errorf("eth2 master = %q", p2.Props["connection.master"])
	}
}

func TestDeleteBridgeFreesPortsThenDeletesThenBringsThemUp(t *testing.T) {
	run := bridgeRunner()
	n := &NetworkManager{Run: run}
	if err := n.DeleteBridge(context.Background(), "br-lan"); err != nil {
		t.Fatal(err)
	}
	got := changes(run.calls)
	free1, free2 := indexOf(got, "modify u-p1 connection.master"), indexOf(got, "modify u-p2 connection.master")
	del, up1 := indexOf(got, "connection delete u-br"), indexOf(got, "connection up u-p1")
	if free1 < 0 || free2 < 0 || del < 0 || up1 < 0 || del < free1 || del < free2 || up1 < del {
		t.Errorf("order: %q", got)
	}
	// Each freed port is brought up on its own device: without one, NetworkManager
	// may pick another interface while this one is not ready yet.
	for _, want := range []string{"connection up u-p1 ifname eth1", "connection up u-p2 ifname eth2"} {
		if indexOf(got, want) < 0 {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if err := n.DeleteBridge(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown bridge: %v", err)
	}
}

// Undoing a deleted bridge makes the bridge again before it puts the ports
// back in it, with the settings they had.
func TestRevertDeletedBridgeRecreatesItBeforeItsPorts(t *testing.T) {
	prep := bridgeRunner()
	undo, err := (&NetworkManager{Run: prep}).PrepareDeleteBridge(context.Background(), "br-lan")
	if err != nil {
		t.Fatal(err)
	}
	if last := undo.Profiles[len(undo.Profiles)-1]; !last.Deleted || last.Name != "br-lan" {
		t.Fatalf("last = %+v", last)
	}
	// After the delete, the bridge no longer exists.
	run := bridgeRunner()
	run.fail = map[string]bool{"nmcli -g connection.uuid connection show br-lan": true}
	if err := (&NetworkManager{Run: run}).Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	got := changes(run.calls)
	add, modify, up := indexOf(got, "connection add type bridge ifname br-lan con-name br-lan"), indexOf(got, "modify br-lan bridge.stp yes"), indexOf(got, "connection up br-lan ifname br-lan")
	port := indexOf(got, "modify u-p1 ")
	if add < 0 || modify < add || up < modify || port < up {
		t.Errorf("order: %q", got)
	}
	if !strings.Contains(got[port], "connection.master br-lan") || !strings.Contains(got[port], "connection.slave-type bridge") {
		t.Errorf("port restore: %q", got[port])
	}
	if indexOf(got, "connection up u-p1 ifname eth1") < port {
		t.Errorf("a port is brought up through the bridge, not reapplied: %q", got)
	}
}

// A bridge that exists already is not made twice when a revert is retried.
func TestRevertDeletedBridgeDoesNotAddItWhenItExists(t *testing.T) {
	undo := Undo{Profiles: []ProfileUndo{{Name: "br-lan", Device: "br-lan", Deleted: true, Props: map[string]string{"bridge.stp": "yes"}}}}
	run := &fakeRunner{}
	if err := (&NetworkManager{Run: run}).Revert(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if indexOf(changes(run.calls), "connection add") >= 0 {
		t.Errorf("added again: %q", run.calls)
	}
}

type missingRunner struct{ calls []string }

func (m *missingRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, []byte, error) {
	m.calls = append(m.calls, name+" "+strings.Join(args, " "))
	return nil, nil, &ExitError{Command: name, Code: 10, Output: "Error: unknown connection 'x'."}
}

// A revert that is retried finds what it deleted already gone.
func TestRevertCreatedProfileAlreadyGoneIsDone(t *testing.T) {
	undo := Undo{Profiles: []ProfileUndo{{Name: "ohmyserver-br-new", Created: true}}}
	if err := (&NetworkManager{Run: &missingRunner{}}).Revert(context.Background(), undo); err != nil {
		t.Errorf("revert: %v", err)
	}
}

func TestBridgeNamesAndPorts(t *testing.T) {
	for _, name := range []string{"br-lan", "br0", "lan"} {
		if err := ValidBridgeName(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"docker0", "br-0123456789ab", "virbr0", "has space", "a-name-longer-than-15", "../x", ""} {
		if ValidBridgeName(name) == nil {
			t.Errorf("%q accepted", name)
		}
	}
	c, err := BridgeConfig{Ports: []string{" eth2", "eth1", "eth2"}}.Normalize()
	if err != nil || !reflect.DeepEqual(c.Ports, []string{"eth1", "eth2"}) {
		t.Errorf("ports = %v, %v", c.Ports, err)
	}
	if _, err := (BridgeConfig{Ports: []string{"eth1;rm"}}).Normalize(); err == nil {
		t.Error("a name with a semicolon was accepted")
	}
}

func TestBridgeProblems(t *testing.T) {
	links := []Link{
		{Name: "eth0", Kind: "ethernet"}, {Name: "eth1", Kind: "ethernet", Master: "br-other"},
		{Name: "wlan0", Kind: "wifi"}, {Name: "docker0", Kind: "bridge"}, {Name: "br-lan", Kind: "bridge"},
		{Name: "veth1", Kind: "ethernet"},
	}
	cases := []struct {
		name    string
		current *Bridge
		ports   []string
		field   string
	}{
		{"br-new", nil, []string{"eth0"}, ""},
		{"br-new", nil, []string{"eth1"}, "ports"},
		{"br-new", nil, []string{"wlan0"}, "ports"},
		{"br-new", nil, []string{"missing"}, "ports"},
		{"br-new", nil, []string{"veth1"}, "ports"},
		{"eth0", nil, nil, "name"},
		{"br-lan", nil, nil, "name"},
		{"br-lan", &Bridge{Name: "br-lan"}, []string{"eth0"}, ""},
	}
	for _, c := range cases {
		fields := BridgeProblems(c.name, c.current, BridgeConfig{Ports: c.ports}, links)
		if c.field == "" && len(fields) > 0 || c.field != "" && fields[c.field] == "" {
			t.Errorf("%s %v: %v", c.name, c.ports, fields)
		}
	}
}
