package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Features is what this machine can do beyond plain addressing, so pages can
// offer Wi-Fi and dial-up where they work and say what is missing where they
// do not.
type Features struct {
	WiFi  []string  `json:"wifi"` // wireless devices NetworkManager manages
	PPPoE Readiness `json:"pppoe"`
}

// Readiness says whether a feature can be used, and what it still needs.
type Readiness struct {
	Ready   bool     `json:"ready"`
	Missing []string `json:"missing,omitempty"`
}

// Lookups the host answers; tests replace them.
var (
	lookPath = exec.LookPath
	glob     = filepath.Glob
	stat     = os.Stat
)

// sbin is where pppd lives on most distributions. A service's PATH often
// leaves it out, so it is searched explicitly.
var sbin = []string{"/usr/sbin", "/sbin", "/usr/local/sbin"}

// nmPPPPlugins are the places NetworkManager's PPP plugin is installed:
// Debian puts it under a multiarch directory, Fedora under lib64.
var nmPPPPlugins = []string{
	"/usr/lib/*/NetworkManager/*/libnm-ppp-plugin.so",
	"/usr/lib/NetworkManager/*/libnm-ppp-plugin.so",
	"/usr/lib64/NetworkManager/*/libnm-ppp-plugin.so",
	"/usr/lib64/NetworkManager/libnm-ppp-plugin.so",
}

// DetectFeatures reports what NetworkManager can do here. It is only
// meaningful with NetworkManager; other managers have none of it.
func (n *NetworkManager) DetectFeatures(ctx context.Context) Features {
	f := Features{WiFi: []string{}}
	if out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", "DEVICE,TYPE", "device"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if device, kind, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(kind) == "wifi" {
				f.WiFi = append(f.WiFi, device)
			}
		}
	}
	f.PPPoE = pppoeReadiness()
	return f
}

func pppoeReadiness() Readiness {
	var missing []string
	if !hasPPPD() {
		missing = append(missing, "pppd")
	}
	if !hasNMPlugin() {
		missing = append(missing, "NetworkManager PPP plugin")
	}
	return Readiness{Ready: len(missing) == 0, Missing: missing}
}

func hasPPPD() bool {
	if _, err := lookPath("pppd"); err == nil {
		return true
	}
	for _, dir := range sbin {
		if _, err := stat(filepath.Join(dir, "pppd")); err == nil {
			return true
		}
	}
	return false
}

func hasNMPlugin() bool {
	for _, pattern := range nmPPPPlugins {
		if matches, err := glob(pattern); err == nil && len(matches) > 0 {
			return true
		}
	}
	return false
}
