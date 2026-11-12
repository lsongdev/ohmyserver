package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recorder answers commands and keeps what each was run with, script
// included, since the ruleset travels on stdin.
type recorder struct {
	calls   []string
	stdin   []string
	fail    map[string]error
	outputs map[string]string
}

func (r *recorder) Run(_ context.Context, stdin string, name string, args ...string) ([]byte, []byte, error) {
	call := filepath.Base(name) + " " + strings.Join(args, " ")
	r.calls = append(r.calls, call)
	r.stdin = append(r.stdin, stdin)
	return []byte(r.outputs[call]), nil, r.fail[call]
}

func stubTools(t *testing.T, tools ...string) {
	t.Helper()
	old := lookTool
	t.Cleanup(func() { lookTool = old })
	lookTool = func(name string) (string, error) {
		for _, have := range tools {
			if have == name {
				return "/usr/sbin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestApplyPassesTheScriptOnStdinAndSetsForwarding(t *testing.T) {
	stubTools(t, "nft", "sysctl")
	r, dir := &recorder{}, t.TempDir()
	f := &Firewall{Run: r, SysctlDir: dir}
	c := Config{Enabled: true, WAN: []string{"eth1"}, Masquerade: true}
	if err := f.Apply(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if r.calls[0] != "nft -f -" || r.stdin[0] != c.Script() {
		t.Errorf("first call %q with %q", r.calls[0], r.stdin[0])
	}
	if r.calls[1] != "sysctl -w net.ipv4.ip_forward=1" {
		t.Errorf("calls = %q", r.calls)
	}
	if text, _ := os.ReadFile(filepath.Join(dir, forwardFile)); !strings.Contains(string(text), "net.ipv4.ip_forward = 1") {
		t.Errorf("forwarding is not kept across reboots: %q", text)
	}
}

// The script goes on stdin, never into an argument.
func TestNoRulesetIsOnACommandLine(t *testing.T) {
	stubTools(t, "nft")
	r := &recorder{}
	f := &Firewall{Run: r, SysctlDir: t.TempDir()}
	f.Apply(context.Background(), Config{Enabled: true, WAN: []string{"eth1"}, Allow: []Port{{"tcp", 22}}})
	f.Check(context.Background(), Config{Enabled: true, WAN: []string{"eth1"}})
	for _, call := range r.calls {
		if strings.Contains(call, "iifname") || strings.Contains(call, "table inet") {
			t.Errorf("script in arguments: %q", call)
		}
	}
	if r.calls[1] != "nft -c -f -" {
		t.Errorf("check = %q", r.calls[1])
	}
}

// Turning the firewall off stops insisting on forwarding, but leaves the
// running kernel as it is: something else may need it.
func TestDisablingDropsTheForwardingFileOnly(t *testing.T) {
	stubTools(t, "nft", "sysctl")
	r, dir := &recorder{}, t.TempDir()
	path := filepath.Join(dir, forwardFile)
	os.WriteFile(path, []byte("x"), 0o644)
	if err := (&Firewall{Run: r, SysctlDir: dir}).Apply(context.Background(), Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file still there: %v", err)
	}
	for _, call := range r.calls {
		if strings.HasPrefix(call, "sysctl") {
			t.Errorf("changed the kernel: %q", call)
		}
	}
}

func TestForwardingFallsBackToProcWithoutSysctl(t *testing.T) {
	stubTools(t, "nft")
	old, proc := procForward, filepath.Join(t.TempDir(), "ip_forward")
	procForward = proc
	t.Cleanup(func() { procForward = old })
	if err := (&Firewall{Run: &recorder{}, SysctlDir: t.TempDir()}).Apply(context.Background(), Config{Enabled: true, WAN: []string{"eth1"}, Masquerade: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(proc); string(got) != "1\n" {
		t.Errorf("proc = %q", got)
	}
}

func TestWithoutNftEverythingSaysSo(t *testing.T) {
	stubTools(t)
	f := &Firewall{Run: &recorder{}}
	if f.Available() {
		t.Error("available")
	}
	for name, err := range map[string]error{
		"check": f.Check(context.Background(), Config{}), "apply": f.Apply(context.Background(), Config{}),
	} {
		if err == nil || !strings.Contains(err.Error(), "nftables") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.Rules(context.Background()); err == nil {
		t.Error("rules without nft")
	}
}

// A table that is not there has no rules; that is not a failure.
func TestRulesOfAMissingTableAreEmpty(t *testing.T) {
	stubTools(t, "nft")
	r := &recorder{fail: map[string]error{"nft list table inet ohmyserver": &ExitError{Command: "nft", Code: 1, Output: "No such file or directory"}}}
	got, err := (&Firewall{Run: r}).Rules(context.Background())
	if err != nil || got != "" {
		t.Errorf("%q, %v", got, err)
	}
}
