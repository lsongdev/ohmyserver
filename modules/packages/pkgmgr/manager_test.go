package pkgmgr

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeExecutor struct {
	output string
	calls  []Command
	err    error
}

func (e *fakeExecutor) Run(_ context.Context, command Command) (Result, error) {
	e.calls = append(e.calls, cloneCommand(command))
	result := Result{Command: command, Stdout: e.output}
	if e.err != nil {
		return result, e.err
	}
	return result, nil
}

func fakeLookPath(available ...string) LookPathFunc {
	set := make(map[string]bool, len(available))
	for _, name := range available {
		set[name] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/mock/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func allLookPath(name string) (string, error) {
	return "/mock/" + name, nil
}

func TestChangeCommands(t *testing.T) {
	tests := []struct {
		name       string
		backend    Backend
		operation  Operation
		packages   []string
		options    ChangeOptions
		executable string
		args       []string
	}{
		{
			name: "apt install", backend: APT, operation: Install,
			packages: []string{"curl"}, options: ChangeOptions{AssumeYes: true},
			executable: "/mock/apt-get", args: []string{"install", "--yes", "curl"},
		},
		{
			name: "dnf remove", backend: DNF, operation: Remove,
			packages: []string{"curl"}, options: ChangeOptions{AssumeYes: true},
			executable: "/mock/dnf5", args: []string{"remove", "--assumeyes", "curl"},
		},
		{
			name: "pacman upgrade all", backend: Pacman, operation: Upgrade,
			options:    ChangeOptions{AssumeYes: true},
			executable: "/mock/pacman", args: []string{"-Syu", "--noconfirm"},
		},
		{
			name: "apk noninteractive option precedes command", backend: APK, operation: Install,
			packages: []string{"curl"}, options: ChangeOptions{AssumeYes: true},
			executable: "/mock/apk", args: []string{"--no-interactive", "add", "curl"},
		},
		{
			name: "opkg explicit upgrade", backend: OPKG, operation: Upgrade,
			packages:   []string{"curl"},
			executable: "/mock/opkg", args: []string{"upgrade", "curl"},
		},
		{
			name: "brew upgrade", backend: Brew, operation: Upgrade,
			packages: []string{"curl"}, options: ChangeOptions{AssumeYes: true},
			executable: "/mock/brew", args: []string{"upgrade", "--no-ask", "curl"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, err := New(test.backend, Options{LookPath: allLookPath})
			if err != nil {
				t.Fatal(err)
			}
			command, err := manager.Command(test.operation, test.packages, test.options)
			if err != nil {
				t.Fatal(err)
			}
			if command.Name != test.executable {
				t.Fatalf("executable = %q, want %q", command.Name, test.executable)
			}
			if !reflect.DeepEqual(command.Args, test.args) {
				t.Fatalf("args = %#v, want %#v", command.Args, test.args)
			}
			if !reflect.DeepEqual(command.Env, []string{"LC_ALL=C", "LANG=C"}) {
				t.Fatalf("env = %#v", command.Env)
			}
		})
	}
}

func TestPackageReferencesCannotBecomeOptionsOrShell(t *testing.T) {
	manager, err := New(APT, Options{LookPath: allLookPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"--purge", "curl;reboot", "curl reboot", "curl\nreboot"} {
		_, err := manager.Command(Install, []string{reference}, ChangeOptions{})
		if err == nil {
			t.Fatalf("expected %q to be rejected", reference)
		}
	}
}

func TestDetectAllRequiresCompleteAPTToolset(t *testing.T) {
	backends, err := DetectAll(Options{
		LookPath: fakeLookPath("apt-get", "brew"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backends, []Backend{Brew}) {
		t.Fatalf("backends = %#v, want brew only", backends)
	}
}

func TestOPKGUpgradeResolvesExplicitPackages(t *testing.T) {
	query := &fakeExecutor{
		output: "curl - 8.0 - 8.1\nluci - 1.0 - 1.1\n",
	}
	mutation := &fakeExecutor{}
	manager, err := New(OPKG, Options{
		LookPath:         allLookPath,
		Executor:         query,
		MutatingExecutor: mutation,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Apply(context.Background(), Upgrade, nil, ChangeOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(query.calls) != 1 || !reflect.DeepEqual(query.calls[0].Args, []string{"list-upgradable"}) {
		t.Fatalf("query calls = %#v", query.calls)
	}
	if len(mutation.calls) != 1 {
		t.Fatalf("mutation calls = %d, want 1", len(mutation.calls))
	}
	want := []string{"upgrade", "curl", "luci"}
	if !reflect.DeepEqual(mutation.calls[0].Args, want) {
		t.Fatalf("upgrade args = %#v, want %#v", mutation.calls[0].Args, want)
	}
}

func TestOPKGUpgradeSkipsWhenCurrent(t *testing.T) {
	query := &fakeExecutor{}
	mutation := &fakeExecutor{}
	manager, err := New(OPKG, Options{
		LookPath:         allLookPath,
		Executor:         query,
		MutatingExecutor: mutation,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := manager.Upgrade(context.Background(), nil, ChangeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Fatal("expected an empty upgrade to be skipped")
	}
	if len(mutation.calls) != 0 {
		t.Fatalf("mutation calls = %d, want 0", len(mutation.calls))
	}
}

func TestCommandStringIsDisplayOnlyAndQuoted(t *testing.T) {
	command := Command{Name: "/usr/bin/example", Args: []string{"plain", "two words", "a'b"}}
	got := command.String()
	if !strings.Contains(got, "'two words'") || !strings.Contains(got, "'a'\"'\"'b'") {
		t.Fatalf("unexpected command string %q", got)
	}
}
