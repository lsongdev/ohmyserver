package system

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// nmProfile is the part of a connection profile that decides what a bridge
// and its ports are.
type nmProfile struct{ UUID, ID, Type, Interface, Master, SlaveType string }

func clean(v string) string {
	if v = strings.TrimSpace(v); v == "--" {
		return ""
	}
	return v
}

const profileFields = "connection.id,connection.type,connection.interface-name,connection.master,connection.slave-type"

func (n *NetworkManager) profiles(ctx context.Context) ([]nmProfile, error) {
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", "UUID", "connection", "show")
	if err != nil {
		return nil, err
	}
	var list []nmProfile
	for _, id := range strings.Fields(string(out)) {
		out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", profileFields, "connection", "show", id)
		if err != nil {
			continue // removed meanwhile
		}
		v := parseTerse(string(out))
		list = append(list, nmProfile{
			UUID: id, ID: v["connection.id"], Type: v["connection.type"], Interface: clean(v["connection.interface-name"]),
			Master: clean(v["connection.master"]), SlaveType: clean(v["connection.slave-type"]),
		})
	}
	return list, nil
}

func findBridge(profiles []nmProfile, name string) *nmProfile {
	for i, p := range profiles {
		if p.Type == "bridge" && p.Interface == name {
			return &profiles[i]
		}
	}
	return nil
}

// portsOf lists the interfaces whose profiles name the bridge as their
// master, by its interface name or by its profile's UUID.
func portsOf(profiles []nmProfile, bridge nmProfile) []string {
	ports := []string{}
	for _, p := range profiles {
		if p.SlaveType == "bridge" && p.Interface != "" && (p.Master == bridge.Interface || p.Master == bridge.UUID) {
			ports = append(ports, p.Interface)
		}
	}
	sort.Strings(ports)
	return slices.Compact(ports)
}

func (n *NetworkManager) readBridge(ctx context.Context, profiles []nmProfile, b nmProfile) Bridge {
	stp := false
	if out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", "bridge.stp", "connection", "show", b.UUID); err == nil {
		stp = parseTerse(string(out))["bridge.stp"] == "yes"
	}
	ports := portsOf(profiles, b)
	return Bridge{
		Name: b.Interface, STP: stp, Ports: ports, Profile: b.ID,
		Version: version(fmt.Sprintf("%s %t %s", b.Interface, stp, strings.Join(ports, ","))),
	}
}

func (n *NetworkManager) Bridges(ctx context.Context) ([]Bridge, error) {
	profiles, err := n.profiles(ctx)
	if err != nil {
		return nil, err
	}
	bridges := []Bridge{}
	for _, p := range profiles {
		if p.Type == "bridge" && p.Interface != "" {
			bridges = append(bridges, n.readBridge(ctx, profiles, p))
		}
	}
	sort.Slice(bridges, func(i, j int) bool { return bridges[i].Name < bridges[j].Name })
	return bridges, nil
}

func (n *NetworkManager) Bridge(ctx context.Context, name string) (*Bridge, error) {
	profiles, err := n.profiles(ctx)
	if err != nil {
		return nil, err
	}
	p := findBridge(profiles, name)
	if p == nil {
		return nil, nil
	}
	b := n.readBridge(ctx, profiles, *p)
	return &b, nil
}

func (n *NetworkManager) nmcli(ctx context.Context, args ...string) error {
	_, _, err := n.Run.Run(ctx, "", "nmcli", args...)
	return err
}

// up activates a profile on its device. Moving a device in or out of a
// bridge cannot be reapplied in place, so this brings the connection up.
func (n *NetworkManager) up(ctx context.Context, uuid, device string) error {
	args := []string{"connection", "up", uuid}
	if device != "" {
		args = append(args, "ifname", device)
	}
	return n.nmcli(ctx, args...)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// snapshot captures a profile's owned settings, with its master and port
// type, so a revert can put the interface back in one modify. The master is
// kept by interface name: a bridge made again gets a new UUID.
func (n *NetworkManager) snapshot(ctx context.Context, p nmProfile, extra ...string) (map[string]string, error) {
	props, err := n.props(ctx, p.UUID)
	if err != nil {
		return nil, err
	}
	saved := map[string]string{}
	for k, v := range props {
		if !strings.HasPrefix(k, "connection.") {
			saved[k] = clean(v)
		}
	}
	if len(extra) > 0 {
		out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", strings.Join(extra, ","), "connection", "show", p.UUID)
		if err != nil {
			return nil, err
		}
		for k, v := range parseTerse(string(out)) {
			saved[k] = clean(v)
		}
	}
	return saved, nil
}

// portUndo captures the profile an interface uses now, or notes that it has
// none and one will be made.
func (n *NetworkManager) portUndo(ctx context.Context, profiles []nmProfile, port string) (ProfileUndo, error) {
	uuid, _, err := n.connection(ctx, port)
	if err != nil {
		return ProfileUndo{}, err
	}
	if uuid == "" {
		return ProfileUndo{Name: createdName(port), Device: port, Created: true}, nil
	}
	var current nmProfile
	for _, p := range profiles {
		if p.UUID == uuid {
			current = p
		}
	}
	current.UUID = uuid
	master := current.Master
	for _, p := range profiles {
		if p.Type == "bridge" && p.UUID == master {
			master = p.Interface
		}
	}
	props, err := n.snapshot(ctx, current)
	if err != nil {
		return ProfileUndo{}, err
	}
	// Their presence, even empty, is what tells a revert this is a port.
	props["connection.master"], props["connection.slave-type"] = master, current.SlaveType
	return ProfileUndo{UUID: uuid, Device: port, Props: props}, nil
}

// bridgeProfileName names the connection made for a bridge.
func bridgeProfileName(bridge string) string { return "ohmyserver-" + bridge }

// PrepareBridge lists the bridge first and its ports after, so a revert,
// which walks backwards, frees the ports before it removes the bridge.
func (n *NetworkManager) PrepareBridge(ctx context.Context, name string, c BridgeConfig) (Undo, error) {
	profiles, err := n.profiles(ctx)
	if err != nil {
		return Undo{}, err
	}
	undo := Undo{Backend: n.Name(), Link: name}
	var current []string
	if b := findBridge(profiles, name); b == nil {
		undo.Profiles = append(undo.Profiles, ProfileUndo{Name: bridgeProfileName(name), Device: name, Created: true})
	} else {
		props, err := n.snapshot(ctx, *b, "bridge.stp")
		if err != nil {
			return Undo{}, err
		}
		undo.Profiles = append(undo.Profiles, ProfileUndo{UUID: b.UUID, Device: name, Props: props})
		current = portsOf(profiles, *b)
	}
	ports := append(slices.Clone(c.Ports), current...)
	sort.Strings(ports)
	for _, port := range slices.Compact(ports) {
		p, err := n.portUndo(ctx, profiles, port)
		if err != nil {
			return Undo{}, err
		}
		undo.Profiles = append(undo.Profiles, p)
	}
	return undo, nil
}

// PrepareDeleteBridge lists the ports first and the bridge last, so a revert
// makes the bridge again before it puts the ports back in it.
func (n *NetworkManager) PrepareDeleteBridge(ctx context.Context, name string) (Undo, error) {
	profiles, err := n.profiles(ctx)
	if err != nil {
		return Undo{}, err
	}
	b := findBridge(profiles, name)
	if b == nil {
		return Undo{}, ErrNotFound
	}
	undo := Undo{Backend: n.Name(), Link: name}
	for _, port := range portsOf(profiles, *b) {
		p, err := n.portUndo(ctx, profiles, port)
		if err != nil {
			return Undo{}, err
		}
		undo.Profiles = append(undo.Profiles, p)
	}
	props, err := n.snapshot(ctx, *b, "bridge.stp")
	if err != nil {
		return Undo{}, err
	}
	undo.Profiles = append(undo.Profiles, ProfileUndo{UUID: b.UUID, Name: b.ID, Device: name, Deleted: true, Props: props})
	return undo, nil
}

// recreate makes a deleted bridge again, with the settings it had.
func (n *NetworkManager) recreate(ctx context.Context, p ProfileUndo) error {
	if _, _, err := n.Run.Run(ctx, "", "nmcli", "-g", "connection.uuid", "connection", "show", p.Name); err != nil {
		if err := n.nmcli(ctx, "connection", "add", "type", "bridge", "ifname", p.Device, "con-name", p.Name); err != nil {
			return err
		}
	}
	if err := n.nmcli(ctx, modifyArgs(p.Name, p.Props)...); err != nil {
		return err
	}
	return n.up(ctx, p.Name, p.Device)
}

// release frees a port: it is an ordinary interface again, asking for an
// address, until its settings are edited.
func (n *NetworkManager) release(ctx context.Context, uuid string) error {
	return n.nmcli(ctx, "connection", "modify", uuid, "connection.master", "", "connection.slave-type", "",
		"ipv4.method", "auto", "ipv6.method", "auto")
}

// enslave makes an interface a port of the bridge, with a profile of its own
// when it has none, and returns the profile to bring up.
func (n *NetworkManager) enslave(ctx context.Context, port, bridge string) (string, error) {
	uuid, _, err := n.connection(ctx, port)
	if err != nil {
		return "", err
	}
	if uuid == "" {
		name := createdName(port)
		return name, n.nmcli(ctx, "connection", "add", "type", "ethernet", "ifname", port, "con-name", name,
			"connection.master", bridge, "connection.slave-type", "bridge")
	}
	return uuid, n.nmcli(ctx, "connection", "modify", uuid, "connection.master", bridge, "connection.slave-type", "bridge")
}

// ApplyBridge makes the bridge and its ports match c. A new bridge has no
// address; that is the interface's own settings, edited afterwards.
func (n *NetworkManager) ApplyBridge(ctx context.Context, name string, c BridgeConfig) error {
	profiles, err := n.profiles(ctx)
	if err != nil {
		return err
	}
	ref, current := bridgeProfileName(name), []string(nil)
	if b := findBridge(profiles, name); b != nil {
		ref, current = b.UUID, portsOf(profiles, *b)
		if err := n.nmcli(ctx, "connection", "modify", ref, "bridge.stp", yesNo(c.STP)); err != nil {
			return err
		}
	} else if err := n.nmcli(ctx, "connection", "add", "type", "bridge", "ifname", name, "con-name", ref,
		"bridge.stp", yesNo(c.STP), "ipv4.method", "disabled", "ipv6.method", "ignore"); err != nil {
		return err
	}
	var freed, freedDev, added []string
	for _, p := range profiles {
		if p.SlaveType == "bridge" && slices.Contains(current, p.Interface) && !slices.Contains(c.Ports, p.Interface) {
			if err := n.release(ctx, p.UUID); err != nil {
				return err
			}
			freed, freedDev = append(freed, p.UUID), append(freedDev, p.Interface)
		}
	}
	var enslaved []string
	for _, port := range c.Ports {
		if slices.Contains(current, port) {
			continue
		}
		uuid, err := n.enslave(ctx, port, name)
		if err != nil {
			return err
		}
		added, enslaved = append(added, uuid), append(enslaved, port)
	}
	if err := n.activate(ctx, name, ref); err != nil {
		return err
	}
	for i, uuid := range added {
		if err := n.up(ctx, uuid, enslaved[i]); err != nil {
			return err
		}
	}
	for i, uuid := range freed {
		if err := n.up(ctx, uuid, freedDev[i]); err != nil {
			return err
		}
	}
	return nil
}

// DeleteBridge frees the bridge's ports, removes the bridge, and brings the
// ports up on their own.
func (n *NetworkManager) DeleteBridge(ctx context.Context, name string) error {
	profiles, err := n.profiles(ctx)
	if err != nil {
		return err
	}
	b := findBridge(profiles, name)
	if b == nil {
		return ErrNotFound
	}
	var freed, freedDev []string
	for _, p := range profiles {
		if p.SlaveType == "bridge" && p.Interface != "" && (p.Master == name || p.Master == b.UUID) {
			if err := n.release(ctx, p.UUID); err != nil {
				return err
			}
			freed, freedDev = append(freed, p.UUID), append(freedDev, p.Interface)
		}
	}
	if err := n.nmcli(ctx, "connection", "delete", b.UUID); err != nil {
		return err
	}
	for i, uuid := range freed {
		if err := n.up(ctx, uuid, freedDev[i]); err != nil {
			return err
		}
	}
	return nil
}
