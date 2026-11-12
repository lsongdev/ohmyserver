package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Firewall applies a Config with nft, and keeps IP forwarding on while it
// needs it.
type Firewall struct {
	Run Runner
	// SysctlDir keeps the setting that makes forwarding survive a reboot;
	// empty is /etc/sysctl.d.
	SysctlDir string
}

// forwardFile is the module's own file in SysctlDir.
const forwardFile = "90-ohmyserver-forward.conf"

// sbin is where nft and sysctl live on most distributions. A service's PATH
// often leaves it out, so it is searched explicitly.
var sbin = []string{"/usr/sbin", "/sbin", "/usr/local/sbin"}

// lookTool finds a program on PATH, or in the sbin directories.
var lookTool = func(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, dir := range sbin {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// Available reports whether nft is installed.
func (f *Firewall) Available() bool {
	_, err := lookTool("nft")
	return err == nil
}

func (f *Firewall) nft(ctx context.Context, script string, args ...string) ([]byte, error) {
	path, err := lookTool("nft")
	if err != nil {
		return nil, errors.New("nft is not installed; install the package nftables")
	}
	out, _, err := f.Run.Run(ctx, script, path, args...)
	return out, err
}

// Check has nft read the script without applying it, so a ruleset it would
// refuse is found before anything changes.
func (f *Firewall) Check(ctx context.Context, c Config) error {
	_, err := f.nft(ctx, c.Script(), "-c", "-f", "-")
	return err
}

// Apply makes the table match c in one transaction, then sets forwarding.
func (f *Firewall) Apply(ctx context.Context, c Config) error {
	if _, err := f.nft(ctx, c.Script(), "-f", "-"); err != nil {
		return err
	}
	return f.setForwarding(ctx, c.Forwarding())
}

// Rules lists the table as it is now. A missing table is no rules.
func (f *Firewall) Rules(ctx context.Context) (string, error) {
	out, err := f.nft(ctx, "", "list", "table", "inet", Table)
	var exit *ExitError
	if errors.As(err, &exit) {
		return "", nil
	}
	return string(out), err
}

func (f *Firewall) sysctlDir() string {
	if f.SysctlDir != "" {
		return f.SysctlDir
	}
	return "/etc/sysctl.d"
}

// setForwarding turns IPv4 forwarding on and keeps it on across reboots, or
// stops insisting on it. It never turns it off in the running kernel: other
// software, such as Docker, may rely on it, and the table forwards nothing
// that was not forwarded before when it is gone.
func (f *Firewall) setForwarding(ctx context.Context, on bool) error {
	path := filepath.Join(f.sysctlDir(), forwardFile)
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(f.sysctlDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte("# Written by OhMyServer: the firewall's NAT forwards IPv4.\nnet.ipv4.ip_forward = 1\n"), 0o644); err != nil {
		return err
	}
	if tool, err := lookTool("sysctl"); err == nil {
		_, _, err = f.Run.Run(ctx, "", tool, "-w", "net.ipv4.ip_forward=1")
		return err
	}
	// Without the tool, the kernel's own file does the same.
	return os.WriteFile(procForward, []byte("1\n"), 0o644)
}

var procForward = "/proc/sys/net/ipv4/ip_forward"

// ForwardingNow reports whether the running kernel forwards IPv4.
func ForwardingNow() bool {
	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	return err == nil && strings.TrimSpace(string(data)) == "1"
}
