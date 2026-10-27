package system

import (
	"context"
	"strconv"
	"strings"
)

// DNF drives dnf, reading the installed packages from rpm.
type DNF struct{ run Runner }

func (*DNF) Name() string { return "dnf" }

// rpmFormat prints one installed package per line.
const rpmFormat = `%{NAME}\t%{EPOCH}\t%{VERSION}-%{RELEASE}\t%{ARCH}\t%{SIZE}\t%{SUMMARY}\n`

func (d *DNF) Installed(ctx context.Context) ([]Package, error) {
	out, err := d.run.Output(ctx, nil, "rpm", "-qa", "--qf", rpmFormat)
	if err != nil {
		return nil, err
	}
	return parseRPM(out), nil
}

func parseRPM(out []byte) []Package {
	var result []Package
	for _, line := range lines(out) {
		f := strings.SplitN(line, "\t", 6)
		if len(f) < 6 || f[0] == "gpg-pubkey" { // imported signing keys
			continue
		}
		version := f[2]
		if f[1] != "(none)" && f[1] != "0" {
			version = f[1] + ":" + version
		}
		size, _ := strconv.ParseInt(f[4], 10, 64)
		arch := f[3]
		if arch == "(none)" {
			arch = ""
		}
		result = append(result, Package{Name: f[0], Version: version, Arch: arch, Size: size, Summary: f[5], Installed: true})
	}
	return result
}

func (d *DNF) Updates(ctx context.Context) ([]Update, error) {
	out, err := d.run.Output(ctx, nil, "dnf", "-q", "check-update")
	if err != nil && exitCode(err) != 100 { // 100: updates are available
		return nil, err
	}
	return parseCheckUpdate(out), nil
}

// parseCheckUpdate reads "name.arch  version  repo" lines, up to the
// "Obsoleting Packages" section.
func parseCheckUpdate(out []byte) []Update {
	var result []Update
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "Obsoleting") {
			break
		}
		f := strings.Fields(line)
		if len(f) != 3 || line[0] == ' ' || strings.HasSuffix(f[0], ":") {
			continue
		}
		name, arch := splitArch(f[0])
		result = append(result, Update{Name: name, Arch: arch, Version: f[1], Repo: f[2]})
	}
	return result
}

// splitArch splits "name.arch".
func splitArch(s string) (name, arch string) {
	if i := strings.LastIndexByte(s, '.'); i > 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

const repoqueryFormat = `%{name}\t%{evr}\t%{arch}\t%{repoid}\t%{summary}\n`

func (d *DNF) Search(ctx context.Context, query string) ([]Package, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	out, err := d.run.Output(ctx, nil, "dnf", "-q", "repoquery", "--latest-limit", "1", "--qf", repoqueryFormat, "*"+query+"*")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var result []Package
	for _, line := range lines(out) {
		f := strings.SplitN(line, "\t", 5)
		if len(f) < 5 || f[2] == "src" || seen[f[0]+"."+f[2]] {
			continue
		}
		seen[f[0]+"."+f[2]] = true
		result = append(result, Package{Name: f[0], Version: f[1], Arch: f[2], Repo: f[3], Summary: f[4]})
	}
	return rankSearch(result, query, 200), nil
}

func (d *DNF) Info(ctx context.Context, name string) (string, error) {
	if err := ValidateNames([]string{name}); err != nil {
		return "", err
	}
	out, err := d.run.Output(ctx, nil, "dnf", "-q", "info", name)
	return strings.TrimSpace(string(out)), err
}

func (d *DNF) Refresh(ctx context.Context, out func(string)) error {
	return d.run.Stream(ctx, nil, out, "dnf", "makecache", "--refresh")
}

func (d *DNF) Install(ctx context.Context, names []string, out func(string)) error {
	return d.change(ctx, "install", names, out)
}

func (d *DNF) Remove(ctx context.Context, names []string, out func(string)) error {
	return d.change(ctx, "remove", names, out)
}

func (d *DNF) Upgrade(ctx context.Context, names []string, out func(string)) error {
	return d.change(ctx, "upgrade", names, out)
}

func (d *DNF) change(ctx context.Context, verb string, names []string, out func(string)) error {
	if err := ValidateNames(names); err != nil {
		return err
	}
	return d.run.Stream(ctx, nil, out, "dnf", append([]string{"-y", verb}, names...)...)
}
