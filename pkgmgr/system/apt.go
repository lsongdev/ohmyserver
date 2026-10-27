package system

import (
	"context"
	"strconv"
	"strings"
)

// APT drives apt-get, reading packages with dpkg-query and apt.
type APT struct{ run Runner }

func (*APT) Name() string { return "apt" }

// aptEnv keeps apt from asking questions: configuration files a package
// changes keep the local version, as the default answer would.
var aptEnv = []string{"DEBIAN_FRONTEND=noninteractive"}

var aptOptions = []string{"-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}

const dpkgFormat = `${db:Status-Abbrev}\t${Package}\t${Version}\t${Architecture}\t${Installed-Size}\t${binary:Summary}\n`

func (a *APT) Installed(ctx context.Context) ([]Package, error) {
	out, err := a.run.Output(ctx, nil, "dpkg-query", "-W", "-f", dpkgFormat)
	if err != nil {
		return nil, err
	}
	return parseDpkg(out), nil
}

func parseDpkg(out []byte) []Package {
	var result []Package
	for _, line := range lines(out) {
		f := strings.SplitN(line, "\t", 6)
		if len(f) < 6 || !strings.HasPrefix(f[0], "ii") { // installed, not just known
			continue
		}
		kib, _ := strconv.ParseInt(f[4], 10, 64)
		result = append(result, Package{Name: f[1], Version: f[2], Arch: f[3], Size: kib * 1024, Summary: f[5], Installed: true})
	}
	return result
}

func (a *APT) Updates(ctx context.Context) ([]Update, error) {
	out, err := a.run.Output(ctx, nil, "apt", "list", "--upgradable")
	if err != nil {
		return nil, err
	}
	var result []Update
	for _, p := range parseAptList(out) {
		result = append(result, Update{Name: p.Name, Arch: p.Arch, Version: p.Version, Repo: p.Repo})
	}
	return result, nil
}

// parseAptList reads `apt list` lines: "name/suite,suite version arch
// [installed,…]".
func parseAptList(out []byte) []Package {
	var result []Package
	for _, line := range lines(out) {
		f := strings.Fields(line)
		name, suites, ok := strings.Cut(f[0], "/")
		if len(f) < 3 || !ok {
			continue // "Listing..." and warnings
		}
		repo, _, _ := strings.Cut(suites, ",")
		p := Package{Name: name, Version: f[1], Arch: f[2], Repo: repo}
		if len(f) > 3 {
			status := strings.Trim(f[3], "[]")
			p.Installed = strings.HasPrefix(status, "installed") || strings.HasPrefix(status, "upgradable")
		}
		result = append(result, p)
	}
	return result
}

func (a *APT) Search(ctx context.Context, query string) ([]Package, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	out, err := a.run.Output(ctx, nil, "apt", "list", "*"+query+"*")
	if err != nil {
		return nil, err
	}
	return rankSearch(parseAptList(out), query, 200), nil
}

func (a *APT) Info(ctx context.Context, name string) (string, error) {
	if err := ValidateNames([]string{name}); err != nil {
		return "", err
	}
	out, err := a.run.Output(ctx, nil, "apt-cache", "show", "--no-all-versions", name)
	return strings.TrimSpace(string(out)), err
}

func (a *APT) Refresh(ctx context.Context, out func(string)) error {
	return a.run.Stream(ctx, aptEnv, out, "apt-get", "update")
}

func (a *APT) Install(ctx context.Context, names []string, out func(string)) error {
	return a.change(ctx, []string{"install"}, names, out)
}

func (a *APT) Remove(ctx context.Context, names []string, out func(string)) error {
	return a.change(ctx, []string{"remove"}, names, out)
}

func (a *APT) Upgrade(ctx context.Context, names []string, out func(string)) error {
	if len(names) == 0 {
		return a.change(ctx, []string{"upgrade"}, nil, out)
	}
	return a.change(ctx, []string{"install", "--only-upgrade"}, names, out)
}

func (a *APT) change(ctx context.Context, verb []string, names []string, out func(string)) error {
	if err := ValidateNames(names); err != nil {
		return err
	}
	args := append(append(append([]string{}, aptOptions...), verb...), names...)
	return a.run.Stream(ctx, aptEnv, out, "apt-get", args...)
}
