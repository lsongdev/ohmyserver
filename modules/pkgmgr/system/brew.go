package system

import (
	"context"
	"encoding/json"
	"strings"
)

// Brew drives Homebrew (macOS, and Linux where no system manager is
// found). Formulae and casks are both packages; Repo tells them apart.
// Homebrew refuses to run as root, so a panel using it runs as the user
// who owns the installation.
type Brew struct{ run Runner }

func (*Brew) Name() string { return "brew" }

// brewEnv keeps brew from updating itself before every command (Refresh
// does that), from asking questions, and from adding hints and colour.
var brewEnv = []string{"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1", "HOMEBREW_NO_COLOR=1", "NONINTERACTIVE=1"}

func (b *Brew) Installed(ctx context.Context) ([]Package, error) {
	out, err := b.run.Output(ctx, brewEnv, "brew", "info", "--json=v2", "--installed")
	if err != nil {
		return nil, err
	}
	return parseBrewInfo(out)
}

func parseBrewInfo(out []byte) ([]Package, error) {
	var doc struct {
		Formulae []struct {
			Name      string `json:"name"`
			Desc      string `json:"desc"`
			Tap       string `json:"tap"`
			Installed []struct {
				Version string `json:"version"`
			} `json:"installed"`
		} `json:"formulae"`
		Casks []struct {
			Token     string `json:"token"`
			Desc      string `json:"desc"`
			Installed string `json:"installed"`
		} `json:"casks"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	var result []Package
	for _, f := range doc.Formulae {
		if len(f.Installed) == 0 {
			continue
		}
		result = append(result, Package{Name: f.Name, Version: f.Installed[len(f.Installed)-1].Version,
			Summary: f.Desc, Repo: f.Tap, Installed: true})
	}
	for _, c := range doc.Casks {
		result = append(result, Package{Name: c.Token, Version: c.Installed, Summary: c.Desc, Repo: "cask", Installed: true})
	}
	return result, nil
}

func (b *Brew) Updates(ctx context.Context) ([]Update, error) {
	// brew outdated exits 1 when something is outdated.
	out, err := b.run.Output(ctx, brewEnv, "brew", "outdated", "--json=v2")
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parseBrewOutdated(out)
}

func parseBrewOutdated(out []byte) ([]Update, error) {
	type item struct {
		Name      string   `json:"name"`
		Installed []string `json:"installed_versions"`
		Current   string   `json:"current_version"`
	}
	var doc struct {
		Formulae []item `json:"formulae"`
		Casks    []item `json:"casks"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	var result []Update
	add := func(list []item, repo string) {
		for _, i := range list {
			u := Update{Name: i.Name, Version: i.Current, Repo: repo}
			if len(i.Installed) > 0 {
				u.Current = i.Installed[len(i.Installed)-1]
			}
			result = append(result, u)
		}
	}
	add(doc.Formulae, "")
	add(doc.Casks, "cask")
	return result, nil
}

func (b *Brew) Search(ctx context.Context, query string) ([]Package, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	installed := map[string]bool{}
	for _, kind := range []string{"--formula", "--cask"} {
		out, _ := b.run.Output(ctx, brewEnv, "brew", "list", kind, "-1")
		for _, name := range lines(out) {
			installed[strings.TrimSpace(name)] = true
		}
	}
	var result []Package
	for _, kind := range []string{"--formula", "--cask"} {
		// brew search exits 1 when nothing matches.
		out, err := b.run.Output(ctx, brewEnv, "brew", "search", kind, query)
		if err != nil && len(out) == 0 {
			continue
		}
		repo := ""
		if kind == "--cask" {
			repo = "cask"
		}
		for _, name := range parseBrewSearch(out) {
			result = append(result, Package{Name: name, Repo: repo, Installed: installed[name]})
		}
	}
	return rankSearch(result, query, 100), nil
}

// parseBrewSearch reads brew search's names, skipping headings
// ("==> Formulae") and notes.
func parseBrewSearch(out []byte) []string {
	var names []string
	for _, line := range lines(out) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "==>") || strings.ContainsAny(line, " \t") {
			continue
		}
		names = append(names, strings.TrimSuffix(line, " ✔"))
	}
	return names
}

func (b *Brew) Info(ctx context.Context, name string) (string, error) {
	if err := ValidateNames([]string{name}); err != nil {
		return "", err
	}
	out, err := b.run.Output(ctx, brewEnv, "brew", "info", name)
	return string(out), err
}

func (b *Brew) Refresh(ctx context.Context, out func(string)) error {
	return b.run.Stream(ctx, brewEnv, out, "brew", "update")
}

func (b *Brew) Install(ctx context.Context, names []string, out func(string)) error {
	return b.change(ctx, out, "install", names)
}

func (b *Brew) Remove(ctx context.Context, names []string, out func(string)) error {
	return b.change(ctx, out, "uninstall", names)
}

func (b *Brew) Upgrade(ctx context.Context, names []string, out func(string)) error {
	return b.change(ctx, out, "upgrade", names)
}

func (b *Brew) change(ctx context.Context, out func(string), verb string, names []string) error {
	if err := ValidateNames(names); err != nil {
		return err
	}
	return b.run.Stream(ctx, brewEnv, out, "brew", append([]string{verb}, names...)...)
}
