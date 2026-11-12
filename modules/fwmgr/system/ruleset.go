// Package system renders and applies one nftables table: protection for the
// interfaces that face the internet, and NAT for what leaves through them.
// The table is the module's own, so nothing else's rules are touched.
package system

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Table is the name of the nftables table this module owns, in the inet
// family (IPv4 and IPv6).
const Table = "ohmyserver"

// Port is a port on this machine that the internet may connect to.
type Port struct {
	Proto string `json:"proto"` // tcp or udp
	Port  int    `json:"port"`
}

// Config is what the firewall does. Only the interfaces named in WAN are
// restricted; traffic on every other interface is left as it is, so Docker,
// the local network and loopback keep working.
type Config struct {
	Enabled bool `json:"enabled"`
	// WAN are the interfaces facing the internet. A trailing * matches any
	// ending, such as ppp* for dial-up connections that do not exist yet.
	WAN []string `json:"wan"`
	// Masquerade translates the addresses of IPv4 traffic leaving through a
	// WAN interface, and turns IP forwarding on.
	Masquerade bool `json:"masquerade"`
	AllowPing  bool `json:"allow_ping"`
	// Allow lists what may start a connection to this machine from the WAN.
	// Replies to what this machine started are always let in.
	Allow []Port `json:"allow"`
}

// ValidationError lists what is wrong with a configuration, by field.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string {
	var parts []string
	for _, m := range e.Fields {
		parts = append(parts, m)
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

var ifName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}\*?$`)

const maxAllowed = 64

// Normalize checks a configuration and returns it in canonical form. Names
// and numbers are rewritten from their parsed form: they end up inside a
// script handed to nft, so nothing else may get there.
func (c Config) Normalize() (Config, error) {
	fields := map[string]string{}
	out := Config{Enabled: c.Enabled, Masquerade: c.Masquerade, AllowPing: c.AllowPing, WAN: []string{}, Allow: []Port{}}
	seen := map[string]bool{}
	for _, name := range c.WAN {
		name = strings.TrimSpace(name)
		switch {
		case !ifName.MatchString(name):
			fields["wan"] = fmt.Sprintf("%q is not an interface name; a trailing * is allowed, such as ppp*", name)
		case matchesLoopback(name):
			fields["wan"] = name + " would match the loopback interface, which this machine talks to itself through"
		case !seen[name]:
			seen[name] = true
			out.WAN = append(out.WAN, name)
		}
	}
	sort.Strings(out.WAN)
	if c.Enabled && len(out.WAN) == 0 && fields["wan"] == "" {
		fields["wan"] = "Choose the interface that faces the internet"
	}
	ports := map[Port]bool{}
	for _, p := range c.Allow {
		switch {
		case p.Proto != "tcp" && p.Proto != "udp":
			fields["allow"] = fmt.Sprintf("%q is not tcp or udp", p.Proto)
		case p.Port < 1 || p.Port > 65535:
			fields["allow"] = fmt.Sprintf("%d is not a port between 1 and 65535", p.Port)
		case !ports[p]:
			ports[p] = true
			out.Allow = append(out.Allow, p)
		}
	}
	if len(out.Allow) > maxAllowed {
		fields["allow"] = fmt.Sprintf("At most %d ports may be allowed", maxAllowed)
	}
	sort.Slice(out.Allow, func(i, j int) bool {
		if out.Allow[i].Proto != out.Allow[j].Proto {
			return out.Allow[i].Proto < out.Allow[j].Proto
		}
		return out.Allow[i].Port < out.Allow[j].Port
	})
	if len(fields) > 0 {
		return Config{}, &ValidationError{Fields: fields}
	}
	return out, nil
}

func matchesLoopback(name string) bool {
	ok, _ := path.Match(name, "lo")
	return ok
}

// icmpv6Needed is what IPv6 cannot work without: errors, and neighbour and
// router discovery.
const icmpv6Needed = "destination-unreachable, packet-too-big, time-exceeded, parameter-problem, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert"

func ports(allowed []Port, proto string) string {
	var list []string
	for _, p := range allowed {
		if p.Proto == proto {
			list = append(list, fmt.Sprint(p.Port))
		}
	}
	return strings.Join(list, ", ")
}

// Script is the nft script that makes the table match c. It declares the
// table and deletes it before defining it again, which replaces it in one
// transaction whether or not it existed. A disabled configuration leaves the
// table gone.
//
// The input and forward chains accept by default: only what arrives on a WAN
// interface is restricted, so nothing about a machine's other interfaces
// changes by turning this on. Hooks of other tables (Docker's, for one) are
// evaluated as well, and a drop in any of them wins.
func (c Config) Script() string {
	var b strings.Builder
	fmt.Fprintf(&b, "table inet %s\ndelete table inet %s\n", Table, Table)
	if !c.Enabled {
		return b.String()
	}
	fmt.Fprintf(&b, "table inet %s {\n", Table)
	b.WriteString("\tchain wan_in {\n")
	b.WriteString("\t\tct state established,related accept\n\t\tct state invalid drop\n")
	if c.AllowPing {
		b.WriteString("\t\ticmp type echo-request accept\n\t\ticmpv6 type echo-request accept\n")
	}
	fmt.Fprintf(&b, "\t\ticmpv6 type { %s } accept\n", icmpv6Needed)
	// Offers from a DHCP server are not part of a connection conntrack knows.
	b.WriteString("\t\tudp sport 67 udp dport 68 accept\n\t\tudp sport 547 udp dport 546 accept\n")
	for _, proto := range []string{"tcp", "udp"} {
		if list := ports(c.Allow, proto); list != "" {
			fmt.Fprintf(&b, "\t\t%s dport { %s } accept\n", proto, list)
		}
	}
	b.WriteString("\t\tdrop\n\t}\n")
	b.WriteString("\tchain input {\n\t\ttype filter hook input priority 0; policy accept;\n")
	for _, name := range c.WAN {
		fmt.Fprintf(&b, "\t\tiifname %q jump wan_in\n", name)
	}
	b.WriteString("\t}\n")
	// Nothing on the internet starts a connection to what is behind this machine.
	b.WriteString("\tchain forward {\n\t\ttype filter hook forward priority 0; policy accept;\n")
	for _, name := range c.WAN {
		fmt.Fprintf(&b, "\t\tiifname %q ct state new drop\n", name)
	}
	b.WriteString("\t}\n")
	if c.Masquerade {
		b.WriteString("\tchain postrouting {\n\t\ttype nat hook postrouting priority 100; policy accept;\n")
		for _, name := range c.WAN {
			fmt.Fprintf(&b, "\t\toifname %q meta nfproto ipv4 masquerade\n", name)
		}
		b.WriteString("\t}\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// Forwarding reports whether the kernel must forward IPv4 for this to work.
func (c Config) Forwarding() bool { return c.Enabled && c.Masquerade }
