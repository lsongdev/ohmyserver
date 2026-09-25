// Package pkgmgr provides a small, dependency-free abstraction over common
// operating-system package managers.
package pkgmgr

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Backend identifies a package manager implementation.
type Backend string

const (
	APT    Backend = "apt"
	DNF    Backend = "dnf"
	Pacman Backend = "pacman"
	APK    Backend = "apk"
	OPKG   Backend = "opkg"
	Brew   Backend = "brew"
)

var (
	ErrUnsupportedBackend = errors.New("unsupported package manager backend")
	ErrNotFound           = errors.New("package manager not found")
	ErrUnsupported        = errors.New("operation is not supported")
	ErrUnsafeOperation    = errors.New("unsafe package manager operation")
)

// Operation identifies a state-changing package manager operation.
type Operation string

const (
	Refresh Operation = "refresh"
	Install Operation = "install"
	Remove  Operation = "remove"
	Upgrade Operation = "upgrade"
	Clean   Operation = "clean"
)

// Package is a package record normalized across package managers. Not every
// backend can populate every field.
type Package struct {
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	LatestVersion string `json:"latest_version,omitempty"`
	Description   string `json:"description,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
	Repository    string `json:"repository,omitempty"`
	Installed     bool   `json:"installed"`
}

// Command is an executable plus an argument vector. It is never interpreted by
// a shell.
type Command struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
}

// String returns a display-only shell-like representation of a command.
func (c Command) String() string {
	parts := make([]string, 0, len(c.Args)+1)
	parts = append(parts, quoteForDisplay(c.Name))
	for _, arg := range c.Args {
		parts = append(parts, quoteForDisplay(arg))
	}
	return strings.Join(parts, " ")
}

func quoteForDisplay(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'\"\\$`;&|<>()[]{}*?!") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// Result captures the output and timing of an executed command.
type Result struct {
	Command  Command       `json:"command"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Skipped  bool          `json:"skipped,omitempty"`
}

// CommandError reports a command that exited unsuccessfully.
type CommandError struct {
	Result Result
	Err    error
}

func (e *CommandError) Error() string {
	message := strings.TrimSpace(e.Result.Stderr)
	if message == "" {
		message = strings.TrimSpace(e.Result.Stdout)
	}
	if message == "" {
		message = e.Err.Error()
	}
	return fmt.Sprintf("%s: %s", e.Result.Command.String(), message)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

// ChangeOptions controls a state-changing operation.
type ChangeOptions struct {
	// AssumeYes requests a non-interactive transaction when supported.
	AssumeYes bool
}

// Capabilities describes the portable features exposed by a backend.
type Capabilities struct {
	Search         bool `json:"search"`
	Info           bool `json:"info"`
	ListInstalled  bool `json:"list_installed"`
	ListUpgradable bool `json:"list_upgradable"`
	Refresh        bool `json:"refresh"`
	Install        bool `json:"install"`
	Remove         bool `json:"remove"`
	Upgrade        bool `json:"upgrade"`
	Clean          bool `json:"clean"`
	SafeUpgradeAll bool `json:"safe_upgrade_all"`
}
