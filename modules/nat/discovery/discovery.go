// Package discovery measures the current IPv4 UDP path from this host.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/nat/stun"
)

// DefaultServers are bootstrap choices; callers can supply their own servers.
func DefaultServers() []string {
	return []string{"stun.miwifi.com:3478", "stun.cloudflare.com:3478", "stun.nextcloud.com:443"}
}

type Config struct {
	Servers      []string `json:"servers"`
	LocalAddress string   `json:"local_address,omitempty"`
}

type Step struct {
	Name           string         `json:"name"`
	Server         string         `json:"server"`
	Target         string         `json:"target,omitempty"`
	ExpectedSource string         `json:"expected_source,omitempty"`
	Local          string         `json:"local,omitempty"`
	Response       *stun.Response `json:"response,omitempty"`
	Error          string         `json:"error,omitempty"`
	Timeout        bool           `json:"timeout,omitempty"`
	DurationMS     int64          `json:"duration_ms"`
}

type Result struct {
	UDP       bool     `json:"udp"`
	Mapping   string   `json:"mapping"`
	Filtering string   `json:"filtering"`
	NATType   string   `json:"nat_type"`
	Local     string   `json:"local,omitempty"`
	Public    []string `json:"public"`
	Notes     []string `json:"notes"`
	Steps     []Step   `json:"steps"`
}

// Detector's hooks keep protocol scenarios testable without public servers.
type Detector struct {
	Resolve  func(context.Context, string) ([]netip.Addr, error)
	Listen   func(context.Context, string) (net.PacketConn, error)
	Exchange func(context.Context, net.PacketConn, netip.AddrPort, netip.AddrPort, uint32, time.Duration) (stun.Response, error)
	Timeout  time.Duration
}

func New() *Detector {
	return &Detector{
		Resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
		},
		Listen: func(ctx context.Context, address string) (net.PacketConn, error) {
			return (&net.ListenConfig{}).ListenPacket(ctx, "udp4", address)
		},
		Exchange: stun.Exchange, Timeout: time.Second,
	}
}

// Run emits immutable progress snapshots. The caller owns its deadline and
// storage. Binding tests share one socket; filtering gets a fresh socket.
func (d *Detector) Run(ctx context.Context, cfg Config, progress func(Result)) Result {
	r := Result{Mapping: "unknown", Filtering: "unknown", NATType: "unknown", Public: []string{}, Notes: []string{}, Steps: []Step{}}
	publish := func() {
		if progress != nil {
			progress(clone(r))
		}
	}
	note := func(s string) {
		for _, n := range r.Notes {
			if n == s {
				return
			}
		}
		r.Notes = append(r.Notes, s)
		publish()
	}
	local := cfg.LocalAddress
	if local == "" {
		local = "0.0.0.0"
	}
	conn, err := d.Listen(ctx, net.JoinHostPort(local, "0"))
	if err != nil {
		note("Cannot open a UDP socket: " + err.Error())
		return r
	}
	defer conn.Close()
	r.Local = conn.LocalAddr().String()
	query := func(c net.PacketConn, name, server string, target, expected netip.AddrPort, flags uint32) (stun.Response, error) {
		start := time.Now()
		reply, err := d.Exchange(ctx, c, target, expected, flags, d.Timeout)
		step := Step{Name: name, Server: server, Target: target.String(), ExpectedSource: expected.String(), Local: c.LocalAddr().String(), DurationMS: time.Since(start).Milliseconds()}
		if err != nil {
			step.Error = err.Error()
			step.Timeout = errors.Is(err, context.DeadlineExceeded)
		} else {
			step.Response = &reply
		}
		r.Steps = append(r.Steps, step)
		publish()
		return reply, err
	}
	type candidate struct {
		server   string
		target   netip.AddrPort
		response stun.Response
	}
	var candidates []candidate
	mappings := map[netip.Addr]netip.AddrPort{}
	for _, server := range cfg.Servers {
		if ctx.Err() != nil {
			break
		}
		host, portText, _ := net.SplitHostPort(server)
		var port uint16
		fmt.Sscan(portText, &port)
		ips, err := d.Resolve(ctx, host)
		if err != nil || len(ips) == 0 {
			detail := "No IPv4 address returned"
			if err != nil {
				detail = err.Error()
			}
			r.Steps = append(r.Steps, Step{Name: "resolve", Server: server, Error: detail})
			publish()
			continue
		}
		ip := ips[0].Unmap()
		if !ip.Is4() {
			continue
		}
		target := netip.AddrPortFrom(ip, port)
		if netip.MustParsePrefix("198.18.0.0/15").Contains(ip) {
			note("DNS returned a benchmarking-range address (possible Fake-IP). Results describe the current network/proxy path.")
		}
		reply, err := query(conn, "binding", server, target, target, 0)
		if err != nil {
			continue
		}
		r.UDP = true
		seen := false
		for _, p := range r.Public {
			if p == reply.Mapped.String() {
				seen = true
			}
		}
		if !seen {
			r.Public = append(r.Public, reply.Mapped.String())
		}
		mappings[ip] = reply.Mapped
		if reply.Origin.IsValid() && reply.Other.IsValid() && reply.Origin.Addr().Is4() && reply.Other.Addr().Is4() && reply.Origin.Addr().IsGlobalUnicast() && reply.Other.Addr().IsGlobalUnicast() && reply.Origin.Addr() != reply.Other.Addr() && reply.Origin.Port() != reply.Other.Port() {
			candidates = append(candidates, candidate{server, reply.Origin, reply})
		} else {
			note(server + ": full RFC 5780 behavior discovery is unavailable (two IP addresses and two ports are required).")
		}
		publish()
	}
	if len(mappings) >= 2 {
		var first netip.AddrPort
		equal := true
		for _, m := range mappings {
			if !first.IsValid() {
				first = m
			} else if first != m {
				equal = false
			}
		}
		if equal {
			r.Mapping = "endpoint_independent_observed"
		} else {
			r.Mapping = "destination_dependent_observed"
		}
	}
	observedMapping := r.Mapping
	for _, c := range candidates {
		if ctx.Err() != nil {
			break
		}
		// Start behavior discovery on a fresh socket. Some transparent proxies
		// rewrite a real server response back to a previously contacted Fake-IP.
		// All mapping probes below still share this new socket.
		behavior, err := d.Listen(ctx, net.JoinHostPort(local, "0"))
		if err != nil {
			note("Cannot open a behavior socket: " + err.Error())
			break
		}
		defer behavior.Close()
		base, err := query(behavior, "behavior_baseline", c.server, c.target, c.target, 0)
		if err != nil || base.Origin != c.target || base.Other != c.response.Other {
			note(c.server + ": direct behavior baseline could not be verified; classification skipped.")
			continue
		}
		other := base.Other
		a, err := query(behavior, "mapping_alternate_ip", c.server, netip.AddrPortFrom(other.Addr(), c.target.Port()), netip.AddrPortFrom(other.Addr(), c.target.Port()), 0)
		if err != nil {
			continue
		}
		if a.Mapped == base.Mapped {
			r.Mapping = "endpoint_independent"
		} else {
			b, err := query(behavior, "mapping_alternate_ip_port", c.server, other, other, 0)
			if err == nil {
				if a.Mapped == b.Mapped {
					r.Mapping = "address_dependent"
				} else {
					r.Mapping = "address_port_dependent"
				}
			}
		}
		publish()
		filter, err := d.Listen(ctx, net.JoinHostPort(local, "0"))
		if err != nil {
			note("Cannot open a fresh filtering socket: " + err.Error())
			break
		}
		fbase, err := query(filter, "filter_baseline", c.server, c.target, c.target, 0)
		if err != nil || fbase.Origin != c.target || fbase.Other != other {
			filter.Close()
			continue
		}
		both, eBoth := query(filter, "filter_change_ip_port", c.server, c.target, other, 6)
		switch {
		case eBoth == nil && both.Mapped == fbase.Mapped:
			r.Filtering = "endpoint_independent"
		case errors.Is(eBoth, context.DeadlineExceeded) && ctx.Err() == nil:
			altPort := netip.AddrPortFrom(c.target.Addr(), other.Port())
			portReply, ePort := query(filter, "filter_change_port", c.server, c.target, altPort, 2)
			// Positive controls ensure alternate endpoints are reachable and the same
			// socket still has its mapping. Timeouts alone do not establish filtering.
			control, eControl := query(filter, "filter_control_alternate", c.server, other, other, 0)
			if eControl == nil && control.Mapped == fbase.Mapped {
				if ePort == nil && portReply.Mapped == fbase.Mapped {
					r.Filtering = "address_dependent"
				} else if errors.Is(ePort, context.DeadlineExceeded) && ctx.Err() == nil {
					controlPort, e := query(filter, "filter_control_port", c.server, altPort, altPort, 0)
					if e == nil && controlPort.Mapped == fbase.Mapped {
						r.Filtering = "address_port_dependent"
						note("Filtering is inferred from repeated CHANGE-REQUEST timeouts with successful alternate-endpoint controls; server behavior or other firewalls can affect this result.")
					}
				}
			}
		}
		filter.Close()
		if r.Filtering == "address_dependent" {
			note("Filtering is inferred from CHANGE-REQUEST results with successful alternate-endpoint controls; server behavior or other firewalls can affect this result.")
		}
		if r.Filtering != "unknown" {
			break
		}
	}
	if observedMapping == "destination_dependent_observed" && r.Mapping == "endpoint_independent" {
		r.Mapping = "inconsistent"
		note("Mapping observations conflict across servers. Routing, proxies or a changing network may be involved; no single NAT type is assigned.")
	}
	if r.UDP && r.Filtering == "unknown" {
		note("Filtering could not be determined: a capable behavior server, matching responses and successful controls are required.")
	}
	switch {
	case r.Mapping == "endpoint_independent" && r.Filtering == "endpoint_independent":
		r.NATType = "full_cone_like"
	case r.Mapping == "endpoint_independent" && r.Filtering == "address_dependent":
		r.NATType = "restricted_cone_like"
	case r.Mapping == "endpoint_independent" && r.Filtering == "address_port_dependent":
		r.NATType = "port_restricted_cone_like"
	case r.Mapping == "address_dependent" || r.Mapping == "address_port_dependent":
		r.NATType = "symmetric_like"
	}
	// A matching local/public endpoint indicates no address translation, but
	// says nothing about inbound firewall reachability.
	if a, e := netip.ParseAddrPort(r.Local); e == nil && a.Addr().IsGlobalUnicast() {
		for _, p := range r.Public {
			if p == a.String() {
				r.NATType = "no_translation_observed"
			}
		}
	}
	if ctx.Err() != nil {
		note("Detection stopped: " + ctx.Err().Error())
	}
	if !r.UDP {
		note("No STUN binding response was received. UDP blocking, routing, DNS and server availability cannot be distinguished by this result alone.")
	}
	note("This measures IPv4 UDP behavior from this server at this time. It does not verify peer-to-peer hole punching or public service reachability.")
	publish()
	return r
}

func clone(r Result) Result {
	r.Public = append([]string{}, r.Public...)
	r.Notes = append([]string{}, r.Notes...)
	r.Steps = append([]Step{}, r.Steps...)
	return r
}

// Validate normalizes settings before any network activity.
func Validate(c *Config) map[string]string {
	fields := map[string]string{}
	if len(c.Servers) == 0 || len(c.Servers) > 5 {
		fields["servers"] = "Enter one to five STUN servers as host:port."
	}
	for i, s := range c.Servers {
		s = strings.TrimSpace(s)
		c.Servers[i] = s
		host, p, err := net.SplitHostPort(s)
		var port int
		if err == nil {
			_, err = fmt.Sscan(p, &port)
		}
		if err != nil || host == "" || strings.ContainsAny(host, " /\\\t\r\n") || port < 1 || port > 65535 || fmt.Sprint(port) != p {
			fields["servers"] = "Use IPv4 addresses or host names with a port, for example stun.example.com:3478."
			break
		}
		if ip, err := netip.ParseAddr(host); err == nil && (!ip.Is4() || ip.IsUnspecified() || ip.IsMulticast()) {
			fields["servers"] = "This detector supports IPv4 unicast STUN servers."
		}
	}
	c.LocalAddress = strings.TrimSpace(c.LocalAddress)
	if c.LocalAddress != "" {
		if ip, err := netip.ParseAddr(c.LocalAddress); err != nil || !ip.Is4() || ip.IsMulticast() {
			fields["local_address"] = "Enter a local IPv4 address, or leave empty for the default route."
		}
	}
	return fields
}
