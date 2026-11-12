package system

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// Check is one step of the connectivity check.
type Check struct {
	ID     string `json:"id"` // route, gateway, dns or internet
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Connectivity is what a machine can reach, from the nearest hop outward.
// Keeping a change that was meant to bring the machine online should rest
// on more than the panel being reachable from the local network.
type Connectivity struct {
	Checks []Check `json:"checks"`
}

// OK reports whether every check passed.
func (c Connectivity) OK() bool {
	for _, check := range c.Checks {
		if !check.OK {
			return false
		}
	}
	return len(c.Checks) > 0
}

// Probe holds the network operations a check uses, so tests can answer them.
type Probe struct {
	Run     Runner
	Dial    func(ctx context.Context, network, address string) (net.Conn, error)
	Lookup  func(ctx context.Context, host string) ([]string, error)
	Targets []string // host:port pairs, any one of which proves the internet is reachable
	Name    string   // looked up to prove DNS works
}

// DefaultProbe returns a probe for this host. The targets are public
// resolvers' addresses, so nothing is looked up to reach them and no
// information about this machine is sent: a TCP connection is opened and
// closed.
func DefaultProbe(run Runner) Probe {
	var d net.Dialer
	return Probe{
		Run:     run,
		Dial:    d.DialContext,
		Lookup:  net.DefaultResolver.LookupHost,
		Targets: []string{"1.1.1.1:443", "8.8.8.8:443", "223.5.5.5:443"},
		Name:    "www.gstatic.com",
	}
}

const checkTimeout = 4 * time.Second

// Check runs the checks in order. A failed step does not stop the rest:
// DNS may work through a local resolver while the route is missing, and
// the person reading the result wants to know which part is broken.
func (p Probe) Check(ctx context.Context) Connectivity {
	var result Connectivity
	add := func(id string, ok bool, detail string) {
		result.Checks = append(result.Checks, Check{ID: id, OK: ok, Detail: detail})
	}

	route, err := p.defaultRoute(ctx)
	switch {
	case err != nil:
		add("route", false, err.Error())
	case route == nil:
		add("route", false, "There is no default route.")
	default:
		add("route", true, "Default route through "+route.Device)
	}

	switch {
	case route == nil:
		add("gateway", false, "No default route to follow.")
	case route.Gateway == "":
		// A point-to-point link (PPP, a tunnel) has no gateway to ask.
		add("gateway", true, route.Device+" is a point-to-point link")
	default:
		add("gateway", p.ping(ctx, route), "Gateway "+route.Gateway)
	}

	dnsCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	addrs, err := p.Lookup(dnsCtx, p.Name)
	cancel()
	if err != nil || len(addrs) == 0 {
		add("dns", false, "Could not look up "+p.Name)
	} else {
		add("dns", true, p.Name+" is "+addrs[0])
	}

	reached := ""
	for _, target := range p.Targets {
		dialCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		conn, err := p.Dial(dialCtx, "tcp", target)
		cancel()
		if err == nil {
			conn.Close()
			reached = target
			break
		}
	}
	if reached != "" {
		add("internet", true, "Connected to "+reached)
	} else {
		add("internet", false, "No connection to "+strings.Join(p.Targets, ", "))
	}
	return result
}

// defaultRoute is the default route with the lowest metric: the one traffic
// takes. It is nil when there is none.
func (p Probe) defaultRoute(ctx context.Context) (*Route, error) {
	routes, err := Routes(ctx, p.Run)
	if err != nil {
		return nil, err
	}
	var best *Route
	for i := range routes {
		r := routes[i]
		if r.Destination != "default" || r.Device == "" || r.Type != "" {
			continue
		}
		if best == nil || r.Metric < best.Metric {
			best = &routes[i]
		}
	}
	return best, nil
}

// ping asks the gateway once. Some gateways ignore ICMP, so a silent one is
// reported, not treated as the end of the check.
func (p Probe) ping(ctx context.Context, r *Route) bool {
	addr, err := netip.ParseAddr(r.Gateway)
	if err != nil {
		return false
	}
	args := []string{"-c", "1", "-W", "2", "-n"}
	if addr.Is6() {
		args = append(args, "-I", r.Device)
	}
	args = append(args, addr.String())
	pingCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	_, _, err = p.Run.Run(pingCtx, "", "ping", args...)
	return err == nil
}

// String is a one-line summary for logs.
func (c Connectivity) String() string {
	var parts []string
	for _, check := range c.Checks {
		parts = append(parts, fmt.Sprintf("%s=%t", check.ID, check.OK))
	}
	return strings.Join(parts, " ")
}
