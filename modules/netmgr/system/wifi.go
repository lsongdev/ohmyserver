// Wi-Fi client: scanning, joining networks and forgetting them, through
// NetworkManager. A network's password is written to a 0600 connection file
// and never passed on a command line, where other users could read it.
package system

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// WifiDevice is a wireless interface NetworkManager manages.
type WifiDevice struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	SSID    string `json:"ssid,omitempty"`    // of the saved network it is connected to
	Profile string `json:"profile,omitempty"` // that network's connection id
	// AP: the radio can serve a network. Hotspot is the name of the one it
	// serves now.
	AP      bool   `json:"ap"`
	Hotspot string `json:"hotspot,omitempty"`
}

// WifiNetwork is a network a scan found. Kind says whether this page can
// join it: open, wpa-psk or sae, and unsupported for the rest (enterprise,
// WEP).
type WifiNetwork struct {
	SSID     string `json:"ssid"`
	Signal   int    `json:"signal"` // 0 to 100
	Band     string `json:"band"`
	Channel  int    `json:"channel"`
	Security string `json:"security"`
	Kind     string `json:"kind"`
	InUse    bool   `json:"in_use"`
	Saved    string `json:"saved,omitempty"` // UUID of the saved network with this name
}

// WifiSaved is a network NetworkManager has a connection for.
type WifiSaved struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	SSID        string `json:"ssid"`
	Device      string `json:"device,omitempty"` // it is pinned to, if so
	Kind        string `json:"kind"`
	Autoconnect bool   `json:"autoconnect"`
	Active      bool   `json:"active"`
}

// WifiRequest asks to join a network.
type WifiRequest struct {
	SSID     string `json:"ssid"`
	Security string `json:"security"` // open, wpa-psk or sae; empty picks by the password
	Password string `json:"password,omitempty"`
	Hidden   bool   `json:"hidden"`
}

// WifiBackend is a network manager that can join Wi-Fi networks.
type WifiBackend interface {
	WifiDevices(ctx context.Context) ([]WifiDevice, error)
	WifiScan(ctx context.Context, device string, rescan bool) ([]WifiNetwork, error)
	WifiSaved(ctx context.Context) ([]WifiSaved, error)
	// PrepareWifi checks the request against what is saved and captures what
	// ConnectWifi will replace on the device.
	PrepareWifi(ctx context.Context, device string, r WifiRequest) (Undo, error)
	ConnectWifi(ctx context.Context, device string, r WifiRequest) error
	ForgetWifi(ctx context.Context, uuid string) error
}

var hexKey = regexp.MustCompile(`^[0-9A-Fa-f]{64}$`)

// printable reports whether s has only visible ASCII and spaces.
func printable(s string) bool {
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

// Normalize checks a request and returns it with the security chosen. The
// SSID and password reach a connection file, so what they may hold is
// narrow: a name is up to 32 bytes without control characters, a password
// 8 to 63 printable characters (or 64 hex digits for WPA-PSK).
func (r WifiRequest) Normalize() (WifiRequest, error) {
	fields := map[string]string{}
	out := WifiRequest{SSID: r.SSID, Security: r.Security, Password: r.Password, Hidden: r.Hidden}
	if n := len(r.SSID); n == 0 || n > 32 || strings.ContainsFunc(r.SSID, func(c rune) bool { return c < 0x20 || c == 0x7f }) {
		fields["ssid"] = "A network name is 1 to 32 characters, without control characters"
	}
	if out.Security == "" {
		out.Security = "open"
		if r.Password != "" {
			out.Security = "wpa-psk"
		}
	}
	switch out.Security {
	case "open":
		if r.Password != "" {
			fields["password"] = "An open network has no password"
		}
	case "wpa-psk", "sae":
		switch n := len(r.Password); {
		case !printable(r.Password):
			fields["password"] = "The password may only have printable ASCII characters"
		case n < 8 || n > 63 && !(out.Security == "wpa-psk" && hexKey.MatchString(r.Password)):
			fields["password"] = "A password is 8 to 63 characters"
		}
	default:
		fields["security"] = fmt.Sprintf("%q cannot be joined here; enterprise and WEP networks are not supported", r.Security)
	}
	if len(fields) > 0 {
		return WifiRequest{}, &ValidationError{Fields: fields}
	}
	return out, nil
}

// wifiKind says how a scan's security column can be joined. WPA3 alone is
// SAE; WPA2 and WPA3 together are joined as WPA2, which every client speaks.
func wifiKind(security string) string {
	s := strings.ToUpper(strings.TrimSpace(security))
	switch {
	case s == "" || s == "--":
		return "open"
	case strings.Contains(s, "802.1X") || strings.Contains(s, "WEP") || strings.Contains(s, "OWE"):
		return "unsupported"
	case strings.Contains(s, "WPA3") && !strings.Contains(s, "WPA2") && !strings.Contains(s, "WPA1"):
		return "sae"
	case strings.Contains(s, "WPA"):
		return "wpa-psk"
	}
	return "unsupported"
}

// wifiBand names the band of a frequency in MHz.
func wifiBand(mhz int) string {
	switch {
	case mhz >= 5925:
		return "6 GHz"
	case mhz >= 4900:
		return "5 GHz"
	case mhz >= 2400:
		return "2.4 GHz"
	}
	return ""
}

// MORE-2
