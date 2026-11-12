package system

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
)

// Config is what can be changed about an interface: the same model for
// every backend, which translates it to its own settings.
type Config struct {
	DHCP4     bool          `json:"dhcp4"`
	DHCP6     bool          `json:"dhcp6"`
	Addresses []string      `json:"addresses"` // static, in CIDR form
	Gateway4  string        `json:"gateway4"`
	Gateway6  string        `json:"gateway6"`
	DNS       []string      `json:"dns"`
	MTU       int           `json:"mtu"`    // 0 leaves it to the driver
	Metric    int           `json:"metric"` // priority of this interface's routes, lowest wins; 0 leaves it to the manager
	Routes    []StaticRoute `json:"routes"`
}

// StaticRoute is a persistent route on an interface.
type StaticRoute struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
}

// Setting is an interface's stored configuration.
type Setting struct {
	Config    Config   `json:"config"`
	Version   string   `json:"version"`             // changes when the stored configuration does
	Target    string   `json:"target"`              // the file or connection holding it
	Exists    bool     `json:"exists"`              // false: saving creates it
	Preserved []string `json:"preserved,omitempty"` // settings the form has no field for, kept as they are
	Reason    string   `json:"reason,omitempty"`    // why it cannot be changed here
	NoMTU     bool     `json:"no_mtu,omitempty"`    // this connection type has no MTU to set
	NoMetric  bool     `json:"no_metric,omitempty"` // this network manager cannot set a route priority
}

// Plan shows what saving a configuration would change, as text.
type Plan struct {
	Target string `json:"target"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// Undo holds what is needed to restore an interface, as JSON, so a change
// can still be undone after the panel restarts.
type Undo struct {
	Backend string `json:"backend"`
	Link    string `json:"link"`
	MTU     int    `json:"mtu,omitempty"` // the link's own: managers may not restore it
	// systemd-networkd
	File    string `json:"file,omitempty"`
	Content []byte `json:"content,omitempty"`
	Existed bool   `json:"existed,omitempty"`
	// NetworkManager: one entry per connection profile the change touches,
	// in the order it touched them. Reverting walks them backwards.
	Profiles []ProfileUndo `json:"profiles,omitempty"`
	// Cleanup lists files that only a revert needs, such as the copy of a
	// connection file before it was changed. They are removed when the change
	// is kept.
	Cleanup []string `json:"cleanup,omitempty"`
}

// ProfileUndo restores one NetworkManager connection profile.
type ProfileUndo struct {
	UUID    string            `json:"uuid,omitempty"`
	Name    string            `json:"name,omitempty"`   // of a created profile, which is deleted to undo
	Device  string            `json:"device,omitempty"` // where a restored profile is reapplied
	Created bool              `json:"created,omitempty"`
	Deleted bool              `json:"deleted,omitempty"` // the change removed it, so reverting adds it back
	Props   map[string]string `json:"props,omitempty"`   // the values to put back
	// File is the connection file of a profile this module writes. Backup is
	// a hidden copy of it from before the change, which a revert puts back and
	// loads; Reup then brings the connection up again, if it was up.
	File   string `json:"file,omitempty"`
	Backup string `json:"backup,omitempty"`
	Reup   bool   `json:"reup,omitempty"`
	// Down: the change brought the connection up, so a revert hangs it up.
	Down bool `json:"down,omitempty"`
}

// Backend is the network manager in charge of this machine's interfaces.
type Backend interface {
	Name() string
	// Read returns the stored configuration; Setting.Reason is set when it
	// cannot be changed.
	Read(ctx context.Context, link Link) (*Setting, error)
	// Plan shows the change without making it.
	Plan(ctx context.Context, link Link, c Config) (*Plan, error)
	// Prepare captures what Apply will replace, before anything changes.
	Prepare(ctx context.Context, link Link) (Undo, error)
	// Apply stores the configuration and makes it take effect.
	Apply(ctx context.Context, link Link, c Config) error
	// Revert restores what Prepare captured and makes it take effect.
	Revert(ctx context.Context, undo Undo) error
	// SetUp brings the interface up or down.
	SetUp(ctx context.Context, name string, up bool) error
}

// Detect finds the running network manager, or nil when neither runs (the
// module then only reads).
func Detect(ctx context.Context, run Runner) Backend {
	if _, err := exec.LookPath("nmcli"); err == nil {
		if out, _, err := run.Run(ctx, "", "nmcli", "-t", "-f", "RUNNING", "general"); err == nil && strings.TrimSpace(string(out)) == "running" {
			return &NetworkManager{Run: run}
		}
	}
	if _, err := exec.LookPath("networkctl"); err == nil {
		if out, _, _ := run.Run(ctx, "", "systemctl", "is-active", "systemd-networkd"); strings.TrimSpace(string(out)) == "active" {
			return &Networkd{Run: run, Dir: "/etc/systemd/network"}
		}
	}
	return nil
}

// SupportsMetric reports whether a network manager can set the priority of
// an interface's routes. systemd-networkd keeps it per route and per DHCP
// lease, which the form has no way to say.
func SupportsMetric(b Backend) bool {
	_, ok := b.(*NetworkManager)
	return ok
}

// restoreMTU puts the link's MTU back; a manager with nothing to say about
// MTU anymore leaves it at the value being reverted.
func restoreMTU(ctx context.Context, run Runner, undo Undo) error {
	if undo.MTU <= 0 {
		return nil
	}
	_, _, err := run.Run(ctx, "", "ip", "link", "set", "dev", undo.Link, "mtu", strconv.Itoa(undo.MTU))
	return err
}

// BackendFor returns the backend that made a change, for undoing it after a
// restart.
func BackendFor(name string, run Runner) (Backend, error) {
	switch name {
	case "NetworkManager":
		return &NetworkManager{Run: run}, nil
	case "systemd-networkd":
		return &Networkd{Run: run, Dir: "/etc/systemd/network"}, nil
	}
	return nil, fmt.Errorf("unknown network manager %q", name)
}

// ValidationError lists what is wrong with a configuration, by form field.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string {
	var parts []string
	for _, message := range e.Fields {
		parts = append(parts, message)
	}
	return strings.Join(parts, "; ")
}

// Normalize checks a configuration and returns it in canonical form. Every
// value is rewritten from its parsed form, so nothing else can reach a
// configuration file or a command.
func (c Config) Normalize() (Config, error) {
	fields := map[string]string{}
	out := Config{DHCP4: c.DHCP4, DHCP6: c.DHCP6, MTU: c.MTU, Metric: c.Metric, Addresses: []string{}, DNS: []string{}, Routes: []StaticRoute{}}
	var prefixes []netip.Prefix
	for _, s := range c.Addresses {
		p, err := netip.ParsePrefix(strings.TrimSpace(s))
		if err != nil {
			fields["addresses"] = fmt.Sprintf("%q is not an address with a prefix length, such as 192.168.1.10/24", s)
			continue
		}
		if p.Addr().Is4() && p.Bits() < 31 && p.Addr() == p.Masked().Addr() {
			fields["addresses"] = fmt.Sprintf("%s is the network's own address, not a host's", p)
			continue
		}
		prefixes = append(prefixes, p)
		out.Addresses = append(out.Addresses, p.String())
	}
	gateway := func(field, s string, four bool) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		a, err := netip.ParseAddr(s)
		if err != nil || a.Is4() != four {
			fields[field] = fmt.Sprintf("%q is not an %s address", s, map[bool]string{true: "IPv4", false: "IPv6"}[four])
			return ""
		}
		return a.String()
	}
	out.Gateway4 = gateway("gateway4", c.Gateway4, true)
	out.Gateway6 = gateway("gateway6", c.Gateway6, false)
	// Without DHCP to bring the subnet, a gateway outside every address is
	// unreachable: the default route would fail and the machine go offline.
	if out.Gateway4 != "" && !c.DHCP4 {
		gw := netip.MustParseAddr(out.Gateway4)
		reachable := false
		for _, p := range prefixes {
			reachable = reachable || p.Contains(gw)
		}
		if !reachable {
			fields["gateway4"] = fmt.Sprintf("%s is outside this interface's addresses", out.Gateway4)
		}
	}
	for _, s := range c.DNS {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !validDNS(s) {
			fields["dns"] = fmt.Sprintf("%q is not an IP address", s)
			continue
		}
		out.DNS = append(out.DNS, s)
	}
	if c.MTU != 0 && (c.MTU < 576 || c.MTU > 9216) {
		fields["mtu"] = "MTU must be between 576 and 9216, or empty"
	}
	if c.Metric < 0 || c.Metric > 65535 {
		fields["metric"] = "Priority must be between 1 and 65535, or empty"
	}
	for _, r := range c.Routes {
		dst, err := netip.ParsePrefix(strings.TrimSpace(r.Destination))
		if err != nil {
			fields["routes"] = fmt.Sprintf("%q is not a network, such as 10.1.0.0/16", r.Destination)
			continue
		}
		route := StaticRoute{Destination: dst.Masked().String()}
		if s := strings.TrimSpace(r.Gateway); s != "" {
			gw, err := netip.ParseAddr(s)
			if err != nil || gw.Is4() != dst.Addr().Is4() {
				fields["routes"] = fmt.Sprintf("%q is not a gateway for %s", s, route.Destination)
				continue
			}
			route.Gateway = gw.String()
		}
		out.Routes = append(out.Routes, route)
	}
	if len(fields) > 0 {
		return Config{}, &ValidationError{Fields: fields}
	}
	return out, nil
}

// validDNS accepts an address, with the port and server name
// systemd-resolved allows after it (1.1.1.1:853#cloudflare-dns.com).
func validDNS(s string) bool {
	host, name, named := strings.Cut(s, "#")
	if named && (name == "" || strings.ContainsAny(name, " \t\r\n=")) {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	_, err := netip.ParseAddrPort(host)
	return err == nil
}

// ErrConflict is a stored configuration that changed after it was read.
var ErrConflict = errors.New("the configuration changed since it was opened; reload and try again")
