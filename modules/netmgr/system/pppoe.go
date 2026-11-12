// PPPoE dial-up: connections that log in to a provider over an Ethernet
// interface, through NetworkManager and pppd. The password is written to a
// 0600 connection file and never put on a command line.
package system

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// PPPoE is a dial-up connection as the page shows it.
type PPPoE struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Parent      string `json:"parent"` // the Ethernet interface it dials through
	Username    string `json:"username"`
	Service     string `json:"service,omitempty"`
	MTU         int    `json:"mtu"`
	Metric      int    `json:"metric"`
	HasPassword bool   `json:"has_password"`
	Autoconnect bool   `json:"autoconnect"`
	Active      bool   `json:"active"`
	Device      string `json:"device,omitempty"` // the ppp interface, while it is up
	// Managed: this module wrote it, so it can change it. Others are only shown.
	Managed bool `json:"managed"`
}

// PPPoERequest makes or changes a connection.
type PPPoERequest struct {
	Name     string `json:"name"`
	Parent   string `json:"parent"`
	Username string `json:"username"`
	// Password is empty to keep the one saved.
	Password string `json:"password,omitempty"`
	Service  string `json:"service,omitempty"`
	MTU      int    `json:"mtu"`    // 0 is 1492
	Metric   int    `json:"metric"` // 0 is 50
}

// PPPoEBackend is a network manager that can dial.
type PPPoEBackend interface {
	PPPoEList(ctx context.Context) ([]PPPoE, error)
	// PreparePPPoE checks the request against what is saved and captures
	// what SavePPPoE will replace.
	PreparePPPoE(ctx context.Context, r PPPoERequest) (Undo, error)
	// SavePPPoE writes the connection and dials it.
	SavePPPoE(ctx context.Context, r PPPoERequest) error
	// PrepareDial and Dial bring a saved connection up; the revert hangs up.
	PrepareDial(ctx context.Context, name string) (Undo, error)
	Dial(ctx context.Context, name string) error
	Hangup(ctx context.Context, name string) error
	ForgetPPPoE(ctx context.Context, name string) error
}

// Defaults: 1492 is the MTU that leaves room for PPPoE's eight bytes on
// Ethernet's 1500; 50 puts the dial-up ahead of an Ethernet connection's
// default 100, since it is the one meant to carry the traffic.
const (
	DefaultPPPoEMTU    = 1492
	DefaultPPPoEMetric = 50
)

var pppoeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,31}$`)

// Normalize checks a request and returns it with the defaults filled in. The
// values reach a connection file, so what they may hold is narrow.
func (r PPPoERequest) Normalize() (PPPoERequest, error) {
	fields := map[string]string{}
	out := PPPoERequest{Name: strings.TrimSpace(r.Name), Parent: r.Parent, Username: r.Username, Password: r.Password, Service: r.Service, MTU: r.MTU, Metric: r.Metric}
	if !pppoeName.MatchString(out.Name) || strings.HasSuffix(out.Name, " ") {
		fields["name"] = "A name is 1 to 32 letters, digits, spaces, dots, dashes or underscores"
	}
	if !ValidName(out.Parent) {
		fields["parent"] = "Choose the Ethernet interface that connects to the modem"
	}
	if n := len(out.Username); n == 0 || n > 128 || !printable(out.Username) {
		fields["username"] = "A user name is 1 to 128 printable ASCII characters"
	}
	if n := len(out.Password); n > 128 || !printable(out.Password) {
		fields["password"] = "A password is up to 128 printable ASCII characters"
	}
	if len(out.Service) > 64 || !printable(out.Service) {
		fields["service"] = "A service name is up to 64 printable ASCII characters"
	}
	if out.MTU == 0 {
		out.MTU = DefaultPPPoEMTU
	}
	if out.MTU < 576 || out.MTU > 1500 {
		fields["mtu"] = "MTU must be between 576 and 1500, or empty"
	}
	if out.Metric == 0 {
		out.Metric = DefaultPPPoEMetric
	}
	if out.Metric < 1 || out.Metric > 65535 {
		fields["metric"] = "Priority must be between 1 and 65535, or empty"
	}
	if len(fields) > 0 {
		return PPPoERequest{}, &ValidationError{Fields: fields}
	}
	return out, nil
}

func pppoeUUID(name string) string { return profileUUID("pppoe", name) }

// pppoeIfname is the name of the ppp interface a connection creates.
// NetworkManager will not activate one without a name; it comes from the
// connection's UUID, so it is short enough for Linux (15 characters), the
// same every time, and different for each connection. It starts with ppp,
// which the firewall's ppp* matches.
func pppoeIfname(uuid string) string {
	id := strings.ReplaceAll(uuid, "-", "")
	return "ppp-" + id[:min(len(id), 8)]
}

// pppoeKeyfile is the connection file. IPv6 is left off: asking the provider
// for it can stall a login that works over IPv4, and it is a choice for later.
func pppoeKeyfile(uuid string, r PPPoERequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[connection]\nid=%s\nuuid=%s\ntype=pppoe\ninterface-name=%s\nautoconnect=true\n\n", keyfileValue(r.Name), uuid, pppoeIfname(uuid))
	fmt.Fprintf(&b, "[pppoe]\nparent=%s\nusername=%s\npassword=%s\n", r.Parent, keyfileValue(r.Username), keyfileValue(r.Password))
	if r.Service != "" {
		fmt.Fprintf(&b, "service=%s\n", keyfileValue(r.Service))
	}
	fmt.Fprintf(&b, "\n[ppp]\nmtu=%d\nlcp-echo-failure=5\nlcp-echo-interval=30\n", r.MTU)
	fmt.Fprintf(&b, "\n[ipv4]\nmethod=auto\nroute-metric=%d\n\n[ipv6]\nmethod=ignore\n", r.Metric)
	return b.String()
}
