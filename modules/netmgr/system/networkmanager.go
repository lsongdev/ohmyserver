package system

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
)

// NetworkManager keeps settings in connection profiles, read and changed
// through nmcli.
type NetworkManager struct {
	Run Runner
	// ConnDir holds connection files; empty is /etc/NetworkManager/system-connections.
	ConnDir string
}

func (*NetworkManager) Name() string { return "NetworkManager" }

// The properties this module owns. MTU's name depends on the connection's
// type, and only these types have one the form can set.
var (
	nmProps   = []string{"ipv4.method", "ipv4.addresses", "ipv4.gateway", "ipv4.dns", "ipv4.routes", "ipv6.method", "ipv6.addresses", "ipv6.gateway", "ipv6.dns", "ipv6.routes", "ipv4.route-metric", "ipv6.route-metric"}
	nmMTUProp = map[string]string{"802-3-ethernet": "802-3-ethernet.mtu", "802-11-wireless": "802-11-wireless.mtu"}
)

// parseTerse reads nmcli -t -e no output: "key:value" lines, where only the
// first colon separates.
func parseTerse(text string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok {
			values[key] = strings.TrimSpace(value)
		}
	}
	return values
}

// connection finds the profile a device uses: the active one, or else a
// saved one bound to the device.
func (n *NetworkManager) connection(ctx context.Context, name string) (uuid, state string, err error) {
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", "GENERAL.STATE,GENERAL.CON-UUID", "device", "show", name)
	if err != nil {
		return "", "", err
	}
	device := parseTerse(string(out))
	state = device["GENERAL.STATE"]
	if uuid = device["GENERAL.CON-UUID"]; uuid != "" {
		return uuid, state, nil
	}
	// No active profile: look for a saved one bound to the device.
	out, _, err = n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", "UUID", "connection", "show")
	if err != nil {
		return "", state, err
	}
	for _, id := range strings.Fields(string(out)) {
		bound, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", "connection.interface-name", "connection", "show", id)
		if err == nil && parseTerse(string(bound))["connection.interface-name"] == name {
			return id, state, nil
		}
	}
	return "", state, nil
}

func (n *NetworkManager) props(ctx context.Context, uuid string) (map[string]string, error) {
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", "connection.id,connection.type,"+strings.Join(nmProps, ","), "connection", "show", uuid)
	if err != nil {
		return nil, err
	}
	props := parseTerse(string(out))
	if mtu := nmMTUProp[props["connection.type"]]; mtu != "" {
		out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", mtu, "connection", "show", uuid)
		if err == nil {
			for k, v := range parseTerse(string(out)) {
				props[k] = v
			}
		}
	}
	return props, nil
}

// routeMetric is the priority set on either family; -1 is NetworkManager's
// "choose by device type", which the form calls empty.
func routeMetric(v4, v6 string) int {
	for _, v := range []string{v4, v6} {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func splitList(v string) []string {
	out := []string{}
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" && s != "--" {
			out = append(out, s)
		}
	}
	return out
}

// parseNMRoutes reads ipv4.routes: "ip[/prefix] [next-hop] [metric] [attr=val]…",
// comma-separated. It reports false for anything richer than the form.
func parseNMRoutes(v string) ([]StaticRoute, bool) {
	routes := []StaticRoute{}
	simple := true
	for _, s := range splitList(v) {
		f := strings.Fields(s)
		route := StaticRoute{Destination: f[0]}
		if len(f) > 1 {
			route.Gateway = f[1]
		}
		if len(f) > 2 {
			simple = false
		}
		routes = append(routes, route)
	}
	return routes, simple
}

func configFromProps(props map[string]string) (Config, []string) {
	c := Config{
		DHCP4: props["ipv4.method"] == "auto", DHCP6: props["ipv6.method"] == "auto" || props["ipv6.method"] == "dhcp",
		Addresses: append(splitList(props["ipv4.addresses"]), splitList(props["ipv6.addresses"])...),
		Gateway4:  props["ipv4.gateway"], Gateway6: props["ipv6.gateway"],
		DNS: append(splitList(props["ipv4.dns"]), splitList(props["ipv6.dns"])...),
	}
	if c.Gateway4 == "--" {
		c.Gateway4 = ""
	}
	if c.Gateway6 == "--" {
		c.Gateway6 = ""
	}
	if mtu := nmMTUProp[props["connection.type"]]; mtu != "" {
		c.MTU, _ = strconv.Atoi(props[mtu]) // "auto" is 0
	}
	c.Metric = routeMetric(props["ipv4.route-metric"], props["ipv6.route-metric"])
	four, simple4 := parseNMRoutes(props["ipv4.routes"])
	six, simple6 := parseNMRoutes(props["ipv6.routes"])
	c.Routes = append(four, six...)
	var preserved []string
	if !simple4 || !simple6 {
		// Metrics and attributes have no field; saving would drop them.
		preserved = append(preserved, "Route metrics and attributes are dropped if you save.")
	}
	return c, preserved
}

// wanted turns c into the owned properties. The IPv6 method stays as it was
// when the form has nothing to say about it, so "disabled" is not turned
// into something else by saving an unrelated change.
func wanted(c Config, current map[string]string) map[string]string {
	var addr4, addr6, dns4, dns6, routes4, routes6 []string
	for _, a := range c.Addresses {
		if netip.MustParsePrefix(a).Addr().Is4() {
			addr4 = append(addr4, a)
		} else {
			addr6 = append(addr6, a)
		}
	}
	for _, d := range c.DNS {
		host, _, _ := strings.Cut(d, "#")
		if a, err := netip.ParseAddr(host); err == nil && !a.Is4() {
			dns6 = append(dns6, d)
		} else {
			dns4 = append(dns4, d)
		}
	}
	for _, r := range c.Routes {
		s := strings.TrimSpace(r.Destination + " " + r.Gateway)
		if netip.MustParsePrefix(r.Destination).Addr().Is4() {
			routes4 = append(routes4, s)
		} else {
			routes6 = append(routes6, s)
		}
	}
	metric := "-1"
	if c.Metric > 0 {
		metric = strconv.Itoa(c.Metric)
	}
	p := map[string]string{
		"ipv4.route-metric": metric, "ipv6.route-metric": metric,
		"ipv4.addresses": strings.Join(addr4, ","), "ipv4.gateway": c.Gateway4,
		"ipv4.dns": strings.Join(dns4, ","), "ipv4.routes": strings.Join(routes4, ","),
		"ipv6.addresses": strings.Join(addr6, ","), "ipv6.gateway": c.Gateway6,
		"ipv6.dns": strings.Join(dns6, ","), "ipv6.routes": strings.Join(routes6, ","),
	}
	switch {
	case c.DHCP4:
		p["ipv4.method"] = "auto"
	case len(addr4) > 0:
		p["ipv4.method"] = "manual"
	default:
		p["ipv4.method"] = "disabled"
	}
	switch {
	case c.DHCP6:
		p["ipv6.method"] = "auto"
	case len(addr6) > 0:
		p["ipv6.method"] = "manual"
	case current["ipv6.method"] != "" && current["ipv6.method"] != "auto" && current["ipv6.method"] != "dhcp" && current["ipv6.method"] != "manual":
		p["ipv6.method"] = current["ipv6.method"]
	default:
		p["ipv6.method"] = "link-local"
	}
	if mtu := nmMTUProp[current["connection.type"]]; mtu != "" {
		if c.MTU > 0 {
			p[mtu] = strconv.Itoa(c.MTU)
		} else {
			p[mtu] = "auto"
		}
	}
	return p
}

// modifyArgs orders properties so nmcli sees valid states in one command:
// the method is checked with the addresses already given in the same call.
func modifyArgs(uuid string, p map[string]string) []string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := []string{"connection", "modify", uuid}
	for _, k := range keys {
		args = append(args, k, p[k])
	}
	return args
}

func describeProps(p map[string]string) string {
	keys := make([]string, 0, len(p))
	for k := range p {
		if k != "connection.id" && k != "connection.type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := p[k]
		if v == "--" {
			v = ""
		}
		fmt.Fprintf(&b, "%s: %s\n", k, v)
	}
	return b.String()
}

func (n *NetworkManager) Read(ctx context.Context, link Link) (*Setting, error) {
	uuid, state, err := n.connection(ctx, link.Name)
	if err != nil {
		return nil, err
	}
	if uuid == "" {
		s := &Setting{Config: Config{Addresses: []string{}, DNS: []string{}, Routes: []StaticRoute{}, DHCP4: true}, Version: version(""), Target: "New connection " + link.Name}
		if strings.Contains(state, "unmanaged") {
			s.Reason = "NetworkManager does not manage this interface."
		} else if link.Kind != "ethernet" {
			s.Reason = "This interface has no connection profile; create one with nmcli first."
		}
		return s, nil
	}
	props, err := n.props(ctx, uuid)
	if err != nil {
		return nil, err
	}
	c, preserved := configFromProps(props)
	return &Setting{Config: c, Version: version(describeProps(props)), Target: "Connection " + props["connection.id"], Exists: true, Preserved: preserved, NoMTU: nmMTUProp[props["connection.type"]] == ""}, nil
}

func (n *NetworkManager) Plan(ctx context.Context, link Link, c Config) (*Plan, error) {
	uuid, _, err := n.connection(ctx, link.Name)
	if err != nil {
		return nil, err
	}
	if uuid == "" {
		return &Plan{Target: "New connection " + link.Name, After: describeProps(wanted(c, map[string]string{"connection.type": "802-3-ethernet"}))}, nil
	}
	props, err := n.props(ctx, uuid)
	if err != nil {
		return nil, err
	}
	owned := wanted(c, props)
	before := map[string]string{}
	for k := range owned {
		before[k] = props[k]
	}
	return &Plan{Target: "Connection " + props["connection.id"], Before: describeProps(before), After: describeProps(owned)}, nil
}

func (n *NetworkManager) Prepare(ctx context.Context, link Link) (Undo, error) {
	uuid, _, err := n.connection(ctx, link.Name)
	if err != nil {
		return Undo{}, err
	}
	undo := Undo{Backend: n.Name(), Link: link.Name, MTU: link.MTU}
	if uuid == "" {
		undo.Profiles = []ProfileUndo{{Name: createdName(link.Name), Device: link.Name, Created: true}}
		return undo, nil
	}
	props, err := n.props(ctx, uuid)
	if err != nil {
		return Undo{}, err
	}
	saved := map[string]string{}
	for k, v := range props {
		if strings.HasPrefix(k, "connection.") {
			continue
		}
		if v == "--" {
			v = ""
		}
		saved[k] = v
	}
	undo.Profiles = []ProfileUndo{{UUID: uuid, Device: link.Name, Props: saved}}
	return undo, nil
}

func (n *NetworkManager) Apply(ctx context.Context, link Link, c Config) error {
	uuid, _, err := n.connection(ctx, link.Name)
	if err != nil {
		return err
	}
	current := map[string]string{"connection.type": "802-3-ethernet"}
	if uuid == "" {
		// The name is ours, so a later revert can find and delete it.
		if _, _, err := n.Run.Run(ctx, "", "nmcli", "connection", "add", "type", "ethernet", "ifname", link.Name, "con-name", createdName(link.Name)); err != nil {
			return err
		}
		uuid = createdName(link.Name)
	} else if current, err = n.props(ctx, uuid); err != nil {
		return err
	}
	if _, _, err := n.Run.Run(ctx, "", "nmcli", modifyArgs(uuid, wanted(c, current))...); err != nil {
		return err
	}
	return n.activate(ctx, link.Name, uuid)
}

// createdName names a connection this module created.
func createdName(link string) string { return "ohmyserver-" + link }

// activate applies a profile: in place when it can (no link flap), by
// reactivating when the device cannot take the change that way.
func (n *NetworkManager) activate(ctx context.Context, name, uuid string) error {
	if _, _, err := n.Run.Run(ctx, "", "nmcli", "device", "reapply", name); err == nil {
		return nil
	}
	_, _, err := n.Run.Run(ctx, "", "nmcli", "connection", "up", uuid, "ifname", name)
	return err
}

func (n *NetworkManager) Revert(ctx context.Context, undo Undo) error {
	for i := len(undo.Profiles) - 1; i >= 0; i-- {
		if err := n.revertProfile(ctx, undo.Profiles[i]); err != nil {
			return err
		}
	}
	return restoreMTU(ctx, n.Run, undo)
}

// revertProfile puts one profile back. A revert is retried after a failure
// part-way, so a profile already gone is not an error.
func (n *NetworkManager) revertProfile(ctx context.Context, p ProfileUndo) error {
	switch {
	case p.Created:
		if err := n.nmcli(ctx, "connection", "delete", p.Name); err != nil && !isUnknownConnection(err) {
			return err
		}
		if p.File != "" {
			if err := os.Remove(p.File); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	case p.Down:
		if err := n.nmcli(ctx, "connection", "down", p.UUID); err != nil && !isUnknownConnection(err) && !strings.Contains(err.Error(), "not an active") {
			return err
		}
		return nil
	case p.Backup != "":
		return n.restoreFile(ctx, p)
	case p.Deleted:
		return n.recreate(ctx, p)
	}
	if len(p.Props) == 0 {
		return n.up(ctx, p.UUID, p.Device)
	}
	if err := n.nmcli(ctx, modifyArgs(p.UUID, p.Props)...); err != nil {
		return err
	}
	// A device cannot be moved in or out of a bridge by reapplying.
	if _, port := p.Props["connection.master"]; port {
		return n.up(ctx, p.UUID, p.Device)
	}
	return n.activate(ctx, p.Device, p.UUID)
}

func isUnknownConnection(err error) bool {
	var exit *ExitError
	return errors.As(err, &exit) && strings.Contains(strings.ToLower(exit.Output), "unknown connection")
}

func (n *NetworkManager) SetUp(ctx context.Context, name string, up bool) error {
	verb := "disconnect"
	if up {
		verb = "connect"
	}
	_, _, err := n.Run.Run(ctx, "", "nmcli", "device", verb, name)
	return err
}

// restoreFile puts a connection file back as it was and loads it. A revert is
// retried after a failure part-way, so a copy already moved back is not an
// error.
func (n *NetworkManager) restoreFile(ctx context.Context, p ProfileUndo) error {
	if err := os.Rename(p.Backup, p.File); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := n.nmcli(ctx, "connection", "load", p.File); err != nil {
		return err
	}
	if p.Reup {
		return n.redial(ctx, p.UUID)
	}
	return nil
}
