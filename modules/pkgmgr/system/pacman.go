package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Pacman drives pacman on Arch Linux and its derivatives.
//
// Arch does not support partial upgrades: syncing the package databases
// and then installing one package without upgrading the rest can break
// the system. So Refresh syncs a copy of the databases kept aside (as
// checkupdates from pacman-contrib does), which only Updates reads;
// installing uses the system's databases, consistent with what is
// installed; and upgrading always upgrades everything (pacman -Syu).
type Pacman struct {
	run Runner
	// dbDir holds the copy of the databases Refresh syncs; empty uses a
	// directory under the system's temporary directory.
	dbDir string
}

func (*Pacman) Name() string { return "pacman" }

// ErrPartialUpgrade refuses to upgrade some packages only.
var ErrPartialUpgrade = errors.New("Arch Linux upgrades the whole system at once: use Upgrade all")

func (p *Pacman) checkDB() string {
	if p.dbDir != "" {
		return p.dbDir
	}
	return filepath.Join(os.TempDir(), "ohmyserver-pacman")
}

func (p *Pacman) Installed(ctx context.Context) ([]Package, error) {
	out, err := p.run.Output(ctx, nil, "pacman", "-Qi")
	if err != nil {
		return nil, err
	}
	return parsePacmanInfo(out), nil
}

// parsePacmanInfo reads pacman -Qi: "Key : value" blocks, one per package,
// separated by blank lines; continuation lines are indented.
func parsePacmanInfo(out []byte) []Package {
	var result []Package
	var cur *Package
	for line := range strings.SplitSeq(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "Name":
			result = append(result, Package{Name: value, Installed: true})
			cur = &result[len(result)-1]
		case "Version":
			if cur != nil {
				cur.Version = value
			}
		case "Description":
			if cur != nil {
				cur.Summary = value
			}
		case "Architecture":
			if cur != nil && value != "any" {
				cur.Arch = value
			}
		case "Installed Size":
			if cur != nil {
				cur.Size = pacmanSize(value)
			}
		}
	}
	return result
}

// pacmanSize reads "9.59 MiB".
func pacmanSize(s string) int64 {
	number, unit, _ := strings.Cut(s, " ")
	n, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0
	}
	scale := map[string]float64{"B": 1, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40}[unit]
	return int64(n * scale)
}

func (p *Pacman) Updates(ctx context.Context) ([]Update, error) {
	args := []string{"-Qu"}
	// The copy Refresh synced; before the first refresh, the system's own
	// (possibly old) databases.
	if _, err := os.Stat(filepath.Join(p.checkDB(), "sync")); err == nil {
		args = append(args, "--dbpath", p.checkDB())
	}
	out, err := p.run.Output(ctx, nil, "pacman", args...)
	if err != nil && exitCode(err) != 1 { // 1: nothing to upgrade
		return nil, err
	}
	return parsePacmanUpdates(out), nil
}

// parsePacmanUpdates reads "name 1.0-1 -> 1.1-1" lines.
func parsePacmanUpdates(out []byte) []Update {
	var result []Update
	for _, line := range lines(out) {
		f := strings.Fields(line)
		if len(f) < 4 || f[2] != "->" {
			continue
		}
		result = append(result, Update{Name: f[0], Current: f[1], Version: f[3]})
	}
	return result
}

func (p *Pacman) Search(ctx context.Context, query string) ([]Package, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	out, err := p.run.Output(ctx, nil, "pacman", "-Ss", query)
	if err != nil && exitCode(err) != 1 { // 1: nothing found
		return nil, err
	}
	return rankSearch(parsePacmanSearch(out), query, 200), nil
}

// parsePacmanSearch reads pacman -Ss: "repo/name version [installed]"
// followed by an indented description.
func parsePacmanSearch(out []byte) []Package {
	var result []Package
	for _, line := range lines(out) {
		if strings.HasPrefix(line, " ") {
			if len(result) > 0 {
				result[len(result)-1].Summary = strings.TrimSpace(line)
			}
			continue
		}
		f := strings.Fields(line)
		repo, name, ok := strings.Cut(f[0], "/")
		if !ok || len(f) < 2 {
			continue
		}
		result = append(result, Package{Name: name, Version: f[1], Repo: repo, Installed: strings.Contains(line, "[installed")})
	}
	return result
}

func (p *Pacman) Info(ctx context.Context, name string) (string, error) {
	if err := ValidateNames([]string{name}); err != nil {
		return "", err
	}
	out, err := p.run.Output(ctx, nil, "pacman", "-Si", name)
	if err != nil { // not in a repository: perhaps installed by hand
		out, err = p.run.Output(ctx, nil, "pacman", "-Qi", name)
	}
	return strings.TrimSpace(string(out)), err
}

// Refresh syncs the copy of the databases Updates reads, leaving the
// system's own untouched (see Pacman).
func (p *Pacman) Refresh(ctx context.Context, out func(string)) error {
	dir := p.checkDB()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	local := filepath.Join(dir, "local")
	if _, err := os.Lstat(local); err != nil {
		if err := os.Symlink("/var/lib/pacman/local", local); err != nil {
			return err
		}
	}
	return p.run.Stream(ctx, nil, out, "pacman", "-Sy", "--dbpath", dir, "--logfile", "/dev/null")
}

func (p *Pacman) Install(ctx context.Context, names []string, out func(string)) error {
	return p.change(ctx, out, []string{"-S", "--needed"}, names)
}

func (p *Pacman) Remove(ctx context.Context, names []string, out func(string)) error {
	return p.change(ctx, out, []string{"-R"}, names)
}

// Upgrade upgrades the whole system; naming packages is refused, since
// Arch does not support partial upgrades.
func (p *Pacman) Upgrade(ctx context.Context, names []string, out func(string)) error {
	if len(names) > 0 {
		return ErrPartialUpgrade
	}
	return p.change(ctx, out, []string{"-Syu"}, nil)
}

func (p *Pacman) change(ctx context.Context, out func(string), verb, names []string) error {
	if err := ValidateNames(names); err != nil {
		return err
	}
	args := append(append(append([]string{}, verb...), "--noconfirm"), names...)
	return p.run.Stream(ctx, nil, out, "pacman", args...)
}
