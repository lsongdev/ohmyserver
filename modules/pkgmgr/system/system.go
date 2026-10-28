// Package system drives the host's package manager through its command-line
// tools: dnf on Fedora and Enterprise Linux (RHEL, Rocky, AlmaLinux, Alibaba
// Cloud Linux, …), apt on Debian and Ubuntu, and Homebrew on macOS.
//
// Commands run with LC_ALL=C so their output parses the same everywhere,
// and never through a shell: package names are validated and passed as
// arguments.
package system

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// Package is an installed or available package.
type Package struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Arch      string `json:"arch,omitempty"`
	Size      int64  `json:"size,omitempty"` // installed size in bytes
	Summary   string `json:"summary,omitempty"`
	Repo      string `json:"repo,omitempty"`
	Installed bool   `json:"installed,omitempty"`
}

// Update is a newer version of an installed package.
type Update struct {
	Name    string `json:"name"`
	Arch    string `json:"arch,omitempty"`
	Version string `json:"version"`           // the newer version
	Current string `json:"current,omitempty"` // the installed version
	Repo    string `json:"repo,omitempty"`
}

// Manager is a package manager. Methods that change the system stream
// their output, one line at a time, to out.
type Manager interface {
	Name() string // "dnf", "apt" or "brew"
	Installed(ctx context.Context) ([]Package, error)
	// Updates lists newer versions from the cached repository metadata;
	// Refresh downloads fresh metadata.
	Updates(ctx context.Context) ([]Update, error)
	// Search finds available packages whose name contains query.
	Search(ctx context.Context, query string) ([]Package, error)
	// Info describes a package as the manager prints it.
	Info(ctx context.Context, name string) (string, error)
	Refresh(ctx context.Context, out func(string)) error
	Install(ctx context.Context, names []string, out func(string)) error
	Remove(ctx context.Context, names []string, out func(string)) error
	// Upgrade upgrades the named packages, or every package when names is
	// empty.
	Upgrade(ctx context.Context, names []string, out func(string)) error
}

// ErrUnsupported means no supported package manager was found.
var ErrUnsupported = errors.New("no supported package manager (dnf, apt or brew) on this system")

// ErrInvalid wraps rejected input, such as a malformed package name.
var ErrInvalid = errors.New("invalid")

// Detect returns the system's package manager, run through run.
func Detect(run Runner) (Manager, error) {
	if _, err := exec.LookPath("dnf"); err == nil {
		return &DNF{run: run}, nil
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		if _, err := exec.LookPath("dpkg-query"); err == nil {
			return &APT{run: run}, nil
		}
	}
	// Homebrew last: on Linux the system's own manager comes first.
	if _, err := exec.LookPath("brew"); err == nil {
		return &Brew{run: run}, nil
	}
	return nil, ErrUnsupported
}

// A name may carry an architecture (bash.x86_64, libc6:amd64) or a version
// (nginx-1.24.0, nginx=1.24.0-1); it never starts with "-", so it cannot
// be taken for an option.
var validName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+:@~=-]*$`)

// ValidateNames checks package names before they reach a command line.
func ValidateNames(names []string) error {
	for _, name := range names {
		if len(name) > 200 || !validName.MatchString(name) {
			return fmt.Errorf("%w package name %q", ErrInvalid, name)
		}
	}
	return nil
}

var validQuery = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*$`)

// validateQuery checks a search query: two to 64 name characters.
func validateQuery(query string) error {
	if len(query) < 2 || len(query) > 64 || !validQuery.MatchString(query) {
		return fmt.Errorf("%w search: use 2 to 64 letters, digits or . _ + -", ErrInvalid)
	}
	return nil
}

// rankSearch orders results by how well their name matches query (exact,
// prefix, then anywhere), then by name, and keeps at most limit.
func rankSearch(results []Package, query string, limit int) []Package {
	query = strings.ToLower(query)
	rank := func(name string) int {
		name = strings.ToLower(name)
		switch {
		case name == query:
			return 0
		case strings.HasPrefix(name, query):
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		ri, rj := rank(results[i].Name), rank(results[j].Name)
		if ri != rj {
			return ri < rj
		}
		return results[i].Name < results[j].Name
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
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
