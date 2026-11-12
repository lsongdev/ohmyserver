// Wi-Fi hotspot: the radio serves a network instead of joining one. It is
// made through NetworkManager, which runs a radio in one mode at a time, so
// starting a hotspot leaves the network the radio had joined. The password
// is written to a 0600 connection file, as for every other connection.
package system

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
)

// WifiAP is a hotspot as the page shows it.
type WifiAP struct {
	UUID        string `json:"uuid"`
	Device      string `json:"device"`
	SSID        string `json:"ssid"`
	Band        string `json:"band"`    // bg is 2.4 GHz, a is 5 GHz
	Channel     int    `json:"channel"` // 0 lets the radio choose
	Hidden      bool   `json:"hidden"`
	Address     string `json:"address,omitempty"` // of the radio, with its prefix
	Bridge      string `json:"bridge,omitempty"`  // the bridge it is a port of, instead
	HasPassword bool   `json:"has_password"`
	Active      bool   `json:"active"`
}

// WifiAPRequest makes or changes a hotspot.
type WifiAPRequest struct {
	SSID     string `json:"ssid"`
	Password string `json:"password,omitempty"` // empty keeps the one saved
	Band     string `json:"band"`
	Channel  int    `json:"channel"`
	Hidden   bool   `json:"hidden"`
	Address  string `json:"address,omitempty"`
	Bridge   string `json:"bridge,omitempty"`
}

// WifiAPBackend is a network manager that can run a hotspot.
type WifiAPBackend interface {
	// WifiAP returns the hotspot made for a radio, nil when there is none.
	WifiAP(ctx context.Context, device string) (*WifiAP, error)
	// PrepareWifiAP checks the request against what is saved and captures
	// what SaveWifiAP replaces, including the network the radio has joined.
	PrepareWifiAP(ctx context.Context, device string, r WifiAPRequest) (Undo, error)
	SaveWifiAP(ctx context.Context, device string, r WifiAPRequest) error
	// StopWifiAP ends the hotspot and keeps it from starting at boot.
	StopWifiAP(ctx context.Context, device string) error
}

func validChannel(band string, channel int) bool {
	if band == "bg" {
		return channel >= 1 && channel <= 13
	}
	return channel >= 36 && channel <= 165
}

// Normalize checks a request. Its values reach a connection file, so what
// they may hold is narrow.
func (r WifiAPRequest) Normalize() (WifiAPRequest, error) {
	fields := map[string]string{}
	out := r
	if n := len(r.SSID); n == 0 || n > 32 || strings.ContainsFunc(r.SSID, func(c rune) bool { return c < 0x20 || c == 0x7f }) {
		fields["ssid"] = "A network name is 1 to 32 characters, without control characters"
	}
	if r.Password != "" && (len(r.Password) < 8 || len(r.Password) > 63 || !printable(r.Password)) {
		fields["password"] = "A password is 8 to 63 printable ASCII characters"
	}
	if out.Band == "" {
		out.Band = "bg"
	}
	switch {
	case out.Band != "bg" && out.Band != "a":
		fields["band"] = "The band is 2.4 GHz or 5 GHz"
	case out.Channel != 0 && !validChannel(out.Band, out.Channel):
		fields["channel"] = "That channel does not exist in this band; empty lets the radio choose"
	}
	switch {
	case r.Address != "" && r.Bridge != "":
		fields["bridge"] = "A hotspot has an address of its own or is a port of a bridge, not both"
	case r.Address == "" && r.Bridge == "":
		fields["address"] = "Give the hotspot an address, such as 192.168.50.1/24, or put it in a bridge"
	case r.Address != "":
		p, err := netip.ParsePrefix(strings.TrimSpace(r.Address))
		if err != nil || !p.Addr().Is4() || p.Bits() > 30 || p.Addr() == p.Masked().Addr() {
			fields["address"] = fmt.Sprintf("%q is not an IPv4 host address with a prefix length, such as 192.168.50.1/24", r.Address)
		} else {
			out.Address = p.String()
		}
	case !ValidName(r.Bridge):
		fields["bridge"] = fmt.Sprintf("%q is not an interface name", r.Bridge)
	}
	if len(fields) > 0 {
		return WifiAPRequest{}, &ValidationError{Fields: fields}
	}
	return out, nil
}

func wifiAPUUID(device string) string { return profileUUID("wifi-ap", device) }

// wifiAPKeyfile is the connection file. The priority puts the hotspot ahead
// of a joined network when both could start at boot. A hotspot with an
// address serves it to IPv4 only; one in a bridge has no settings of its own.
func wifiAPKeyfile(uuid, device string, r WifiAPRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[connection]\nid=%s\nuuid=%s\ntype=wifi\ninterface-name=%s\nautoconnect=true\nautoconnect-priority=10\n", keyfileValue(r.SSID), uuid, device)
	if r.Bridge != "" {
		fmt.Fprintf(&b, "master=%s\nslave-type=bridge\n", r.Bridge)
	}
	fmt.Fprintf(&b, "\n[wifi]\nmode=ap\nssid=%s\nband=%s\n", keyfileValue(r.SSID), r.Band)
	if r.Channel > 0 {
		fmt.Fprintf(&b, "channel=%d\n", r.Channel)
	}
	if r.Hidden {
		b.WriteString("hidden=true\n")
	}
	fmt.Fprintf(&b, "\n[wifi-security]\nkey-mgmt=wpa-psk\nproto=rsn\npairwise=ccmp\ngroup=ccmp\npsk=%s\n", keyfileValue(r.Password))
	if r.Address != "" {
		fmt.Fprintf(&b, "\n[ipv4]\nmethod=manual\naddress1=%s\n\n[ipv6]\nmethod=ignore\n", r.Address)
	}
	return b.String()
}
