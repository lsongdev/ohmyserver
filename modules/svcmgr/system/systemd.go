// Package system drives systemd through systemctl and journalctl: listing
// services, showing their state and unit files, starting and stopping them,
// and reading the journal.
//
// Commands run with LC_ALL=C so their output parses the same everywhere,
// and never through a shell: unit names are validated and passed as
// arguments. Output formats are those of systemd 239 (Enterprise Linux 8)
// and later.
package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrUnsupported means systemd is not running this system.
var ErrUnsupported = errors.New("systemd is not running on this system")

// ErrInvalid wraps rejected input, such as a malformed unit name.
var ErrInvalid = errors.New("invalid")

// Running reports whether systemd manages this system (sd_booted).
func Running(root string) bool {
	_, err := os.Stat(root + "/run/systemd/system")
	return err == nil
}

// A service unit name: letters, digits and :_.@- plus the \x escapes
// systemd uses, ending in .service; never starting with "-".
var validUnit = regexp.MustCompile(`^[A-Za-z0-9_\\][A-Za-z0-9:_.@\\-]*\.service$`)

// UnitName validates a service name, adding ".service" when it is missing.
func UnitName(name string) (string, error) {
	if !strings.HasSuffix(name, ".service") {
		name += ".service"
	}
	if len(name) > 256 || !validUnit.MatchString(name) || strings.Contains(name, "@.") {
		return "", fmt.Errorf("%w service name %q", ErrInvalid, name)
	}
	return name, nil
}

// Systemd runs systemctl and journalctl.
type Systemd struct{ Run Runner }

// Unit is a service as systemctl lists it.
type Unit struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Load        string `json:"load"`              // loaded, not-found, masked, …
	Active      string `json:"active"`            // active, inactive, failed, activating, …
	Sub         string `json:"sub"`               // running, exited, dead, …
	Enabled     string `json:"enabled,omitempty"` // the unit file state: enabled, disabled, static, masked, …
}

// Services lists loaded services and the installed ones that are not
// loaded (typically disabled), by name. Templates (name@.service) are left
// out: they run only as instances.
func (s *Systemd) Services(ctx context.Context) ([]Unit, error) {
	out, err := s.Run.Output(ctx, nil, "systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain")
	if err != nil {
		return nil, err
	}
	units := map[string]*Unit{}
	for _, u := range parseListUnits(out) {
		units[u.Name] = &u
	}
	files, err := s.Run.Output(ctx, nil, "systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager")
	if err != nil {
		return nil, err
	}
	for name, state := range parseUnitFiles(files) {
		if strings.Contains(name, "@.") {
			continue
		}
		if u, ok := units[name]; ok {
			u.Enabled = state
		} else {
			units[name] = &Unit{Name: name, Load: "loaded", Active: "inactive", Sub: "dead", Enabled: state}
		}
	}
	result := make([]Unit, 0, len(units))
	for _, u := range units {
		// Aliases repeat the unit they name; not-found units are names other
		// units refer to, with nothing installed behind them.
		if u.Enabled == "alias" || (u.Load == "not-found" && u.Active == "inactive") {
			continue
		}
		result = append(result, *u)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// parseListUnits reads "UNIT LOAD ACTIVE SUB DESCRIPTION" lines. Failed
// units may be marked with a leading "●".
func parseListUnits(out []byte) []Unit {
	var result []Unit
	for _, line := range lines(out) {
		f := strings.Fields(strings.TrimLeft(line, "● *"))
		if len(f) < 4 || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		result = append(result, Unit{Name: f[0], Load: f[1], Active: f[2], Sub: f[3], Description: strings.Join(f[4:], " ")})
	}
	return result
}

// parseUnitFiles reads "UNIT STATE [PRESET]" lines.
func parseUnitFiles(out []byte) map[string]string {
	result := map[string]string{}
	for _, line := range lines(out) {
		if f := strings.Fields(line); len(f) >= 2 && strings.HasSuffix(f[0], ".service") {
			result[f[0]] = f[1]
		}
	}
	return result
}

// Detail is a service's state.
type Detail struct {
	Unit
	Preset     string     `json:"preset,omitempty"`
	Path       string     `json:"path,omitempty"` // the unit file
	MainPID    int        `json:"main_pid,omitempty"`
	Since      *time.Time `json:"since,omitempty"` // when it entered its active state
	Memory     int64      `json:"memory,omitempty"`
	CPU        int64      `json:"cpu_ns,omitempty"`
	Tasks      int        `json:"tasks,omitempty"`
	Restarts   int        `json:"restarts"`
	Result     string     `json:"result,omitempty"` // success, exit-code, signal, timeout, …
	ExitStatus int        `json:"exit_status"`
}

var showProperties = "Id,Description,LoadState,ActiveState,SubState,UnitFileState,UnitFilePreset,FragmentPath," +
	"MainPID,ActiveEnterTimestamp,InactiveEnterTimestamp,MemoryCurrent,CPUUsageNSec,TasksCurrent,NRestarts,Result,ExecMainStatus"

// Show returns a service's state.
func (s *Systemd) Show(ctx context.Context, name string) (*Detail, error) {
	out, err := s.Run.Output(ctx, nil, "systemctl", "show", name, "--no-pager", "--property="+showProperties)
	if err != nil {
		return nil, err
	}
	return parseShow(out), nil
}

func parseShow(out []byte) *Detail {
	p := map[string]string{}
	for _, line := range lines(out) {
		if key, value, ok := strings.Cut(line, "="); ok {
			p[key] = value
		}
	}
	number := func(key string) int64 {
		n, err := strconv.ParseInt(p[key], 10, 64)
		if err != nil || n < 0 || uint64(n) == ^uint64(0) { // "[not set]" or infinity
			return 0
		}
		return n
	}
	d := &Detail{
		Unit: Unit{Name: p["Id"], Description: p["Description"], Load: p["LoadState"], Active: p["ActiveState"],
			Sub: p["SubState"], Enabled: p["UnitFileState"]},
		Preset: p["UnitFilePreset"], Path: p["FragmentPath"], MainPID: int(number("MainPID")),
		Memory: number("MemoryCurrent"), CPU: number("CPUUsageNSec"), Tasks: int(number("TasksCurrent")),
		Restarts: int(number("NRestarts")), Result: p["Result"], ExitStatus: int(number("ExecMainStatus")),
	}
	since := p["ActiveEnterTimestamp"]
	if d.Active == "inactive" || d.Active == "failed" {
		since = p["InactiveEnterTimestamp"]
	}
	if t, ok := parseTimestamp(since); ok {
		d.Since = &t
	}
	return d
}

// parseTimestamp reads systemctl's "Thu 2026-10-22 13:27:19 CST", in the
// local time zone the zone abbreviation names.
func parseTimestamp(s string) (time.Time, bool) {
	if s == "" || s == "n/a" {
		return time.Time{}, false
	}
	for _, layout := range []string{"Mon 2006-01-02 15:04:05 MST", "Mon 2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// File returns the unit file with its drop-ins, as systemctl cat prints it.
func (s *Systemd) File(ctx context.Context, name string) (string, error) {
	out, err := s.Run.Output(ctx, nil, "systemctl", "cat", "--no-pager", name)
	return strings.TrimSpace(string(out)), err
}

// Guarded are services that change the whole system's state when started:
// shutting it down, rebooting, or switching to rescue mode. Act refuses to
// start or restart them; that is a decision for a shell.
var Guarded = map[string]bool{
	"systemd-halt.service": true, "systemd-poweroff.service": true, "systemd-reboot.service": true,
	"systemd-kexec.service": true, "systemd-soft-reboot.service": true, "systemd-exit.service": true,
	"emergency.service": true, "rescue.service": true, "debug-shell.service": true,
}

// ErrGuarded refuses starting a service in Guarded.
var ErrGuarded = errors.New("starting this service shuts down, reboots or switches the system to rescue mode")

// Actions are what Act accepts.
var Actions = []string{"start", "stop", "restart", "reload", "enable", "disable"}

// Act runs a systemctl action on a service and waits for it to finish,
// unless noBlock is set (needed to restart the process that asks).
func (s *Systemd) Act(ctx context.Context, name, action string, noBlock bool) error {
	valid := false
	for _, a := range Actions {
		valid = valid || a == action
	}
	if !valid {
		return fmt.Errorf("%w action %q", ErrInvalid, action)
	}
	if Guarded[name] && (action == "start" || action == "restart" || action == "reload") {
		return fmt.Errorf("%w: use a shell", ErrGuarded)
	}
	args := []string{action}
	if noBlock {
		args = append(args, "--no-block")
	}
	_, err := s.Run.Output(ctx, nil, "systemctl", append(args, name)...)
	return err
}

// SelfUnit returns the service running this process, from its cgroup, or
// "" when it does not run as a service.
func SelfUnit(root string) string {
	data, err := os.ReadFile(root + "/proc/self/cgroup")
	if err != nil {
		return ""
	}
	return parseSelfUnit(string(data))
}

func parseSelfUnit(cgroup string) string {
	for line := range strings.SplitSeq(cgroup, "\n") {
		// cgroup v1 "1:name=systemd:/system.slice/x.service", v2 "0::/system.slice/x.service"
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || (parts[1] != "name=systemd" && parts[1] != "") {
			continue
		}
		for segment := range strings.SplitSeq(parts[2], "/") {
			if strings.HasSuffix(segment, ".service") {
				return segment
			}
		}
	}
	return ""
}

// Version returns systemd's version number, e.g. 239.
func (s *Systemd) Version(ctx context.Context) int {
	out, err := s.Run.Output(ctx, nil, "systemctl", "--version")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}

// lines splits command output into its non-empty lines.
func lines(out []byte) []string {
	var result []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimRight(line, "\r"); strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}
