package pkgmgr

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

func parseTabPackages(output string, installed, versionIsLatest bool) ([]Package, error) {
	var packages []Package
	for _, line := range lines(output) {
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 2 {
			continue
		}
		item := Package{Name: fields[0], Installed: installed}
		if versionIsLatest {
			item.LatestVersion = fields[1]
		} else {
			item.Version = fields[1]
		}
		if len(fields) > 2 {
			item.Architecture = fields[2]
		}
		if len(fields) > 3 {
			item.Description = fields[3]
		}
		packages = append(packages, item)
	}
	return packages, nil
}

func parseSpacePackages(output string, installed bool) []Package {
	var packages []Package
	for _, line := range lines(output) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		packages = append(packages, Package{
			Name: fields[0], Version: fields[1], Installed: installed,
		})
	}
	return packages
}

func parseAPTUpgradable(output string) []Package {
	var packages []Package
	for _, line := range lines(output) {
		if strings.HasPrefix(line, "Listing...") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		nameAndRepo := strings.SplitN(fields[0], "/", 2)
		item := Package{
			Name:          nameAndRepo[0],
			LatestVersion: fields[1],
			Architecture:  fields[2],
			Installed:     true,
		}
		if len(nameAndRepo) > 1 {
			item.Repository = nameAndRepo[1]
		}
		const marker = "upgradable from: "
		if index := strings.Index(line, marker); index >= 0 {
			old := line[index+len(marker):]
			item.Version = strings.TrimSuffix(strings.TrimSpace(old), "]")
		}
		packages = append(packages, item)
	}
	return packages
}

func parseDashSearch(output string) []Package {
	var packages []Package
	for _, line := range lines(output) {
		name, description, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		packages = append(packages, Package{
			Name: strings.TrimSpace(name), Description: strings.TrimSpace(description),
		})
	}
	return packages
}

func parsePacmanUpgradable(output string) []Package {
	var packages []Package
	for _, line := range lines(output) {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[2] != "->" {
			continue
		}
		packages = append(packages, Package{
			Name: fields[0], Version: fields[1], LatestVersion: fields[3], Installed: true,
		})
	}
	return packages
}

func parsePacmanSearch(output string) []Package {
	var packages []Package
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if unicode.IsSpace(rune(raw[0])) {
			if len(packages) > 0 {
				packages[len(packages)-1].Description = strings.TrimSpace(raw)
			}
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) < 2 {
			continue
		}
		nameAndRepo := strings.SplitN(fields[0], "/", 2)
		item := Package{Name: nameAndRepo[len(nameAndRepo)-1], Version: fields[1]}
		if len(nameAndRepo) == 2 {
			item.Repository = nameAndRepo[0]
		}
		item.Installed = strings.Contains(raw, "[installed")
		packages = append(packages, item)
	}
	return packages
}

func parseColonParagraphs(output string) []Package {
	fields := parseKeyValueParagraphs(output, ":")
	packages := make([]Package, 0, len(fields))
	for _, values := range fields {
		item := Package{
			Name:         values["Name"],
			Version:      values["Version"],
			Description:  values["Description"],
			Architecture: values["Architecture"],
			Repository:   values["Repository"],
		}
		if item.Name != "" {
			packages = append(packages, item)
		}
	}
	return packages
}

func parseAPKList(output string, installed, versionIsLatest bool) []Package {
	var packages []Package
	for _, line := range lines(output) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name, version := splitAPKNameVersion(fields[0])
		if name == "" {
			continue
		}
		item := Package{Name: name, Installed: installed}
		if versionIsLatest {
			item.LatestVersion = version
		} else {
			item.Version = version
		}
		if len(fields) > 1 {
			item.Architecture = fields[1]
		}
		packages = append(packages, item)
	}
	return packages
}

func parseAPKSearch(output string) []Package {
	var packages []Package
	for _, line := range lines(output) {
		record, description, _ := strings.Cut(line, " - ")
		name, version := splitAPKNameVersion(record)
		if name == "" {
			continue
		}
		packages = append(packages, Package{
			Name: name, Version: version, Description: strings.TrimSpace(description),
		})
	}
	return packages
}

func parseAPKVersionLines(output string) []Package {
	var packages []Package
	for _, line := range lines(output) {
		name, version := splitAPKNameVersion(strings.Fields(line)[0])
		if name != "" {
			packages = append(packages, Package{Name: name, Version: version})
		}
	}
	return packages
}

func splitAPKNameVersion(value string) (string, string) {
	for index := 0; index < len(value)-1; index++ {
		if value[index] == '-' && value[index+1] >= '0' && value[index+1] <= '9' {
			return value[:index], value[index+1:]
		}
	}
	return value, ""
}

func parseOPKGList(output string, installed, versionIsLatest bool) []Package {
	var packages []Package
	for _, line := range lines(output) {
		fields := strings.Split(line, " - ")
		if len(fields) < 2 {
			continue
		}
		item := Package{Name: strings.TrimSpace(fields[0]), Installed: installed}
		if versionIsLatest && len(fields) >= 3 {
			item.Version = strings.TrimSpace(fields[1])
			item.LatestVersion = strings.TrimSpace(fields[2])
		} else {
			item.Version = strings.TrimSpace(fields[1])
			if len(fields) >= 3 {
				item.Description = strings.TrimSpace(strings.Join(fields[2:], " - "))
			}
		}
		packages = append(packages, item)
	}
	return packages
}

type controlMapping struct {
	name, version, description, architecture, repository string
}

func parseControlParagraphs(output string, mapping controlMapping) []Package {
	fields := parseKeyValueParagraphs(output, ":")
	packages := make([]Package, 0, len(fields))
	for _, values := range fields {
		item := Package{
			Name:         values[mapping.name],
			Version:      values[mapping.version],
			Description:  values[mapping.description],
			Architecture: values[mapping.architecture],
			Repository:   values[mapping.repository],
		}
		if item.Name != "" {
			packages = append(packages, item)
		}
	}
	return packages
}

func parseKeyValueParagraphs(output, separator string) []map[string]string {
	var records []map[string]string
	current := make(map[string]string)
	lastKey := ""
	flush := func() {
		if len(current) > 0 {
			records = append(records, current)
			current = make(map[string]string)
			lastKey = ""
		}
	}
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			flush()
			continue
		}
		if unicode.IsSpace(rune(raw[0])) && lastKey != "" {
			current[lastKey] = strings.TrimSpace(current[lastKey] + " " + strings.TrimSpace(raw))
			continue
		}
		key, value, ok := strings.Cut(raw, separator)
		if !ok {
			continue
		}
		lastKey = strings.TrimSpace(key)
		current[lastKey] = strings.TrimSpace(value)
	}
	flush()
	return records
}

type brewDocument struct {
	Formulae []brewFormula `json:"formulae"`
	Casks    []brewCask    `json:"casks"`
}

type brewFormula struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Desc     string `json:"desc"`
	Versions struct {
		Stable string `json:"stable"`
	} `json:"versions"`
	Installed []struct {
		Version string `json:"version"`
	} `json:"installed"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
}

type brewCask struct {
	Token             string    `json:"token"`
	Name              brewNames `json:"name"`
	Desc              string    `json:"desc"`
	Version           string    `json:"version"`
	Installed         string    `json:"installed"`
	InstalledVersions []string  `json:"installed_versions"`
	CurrentVersion    string    `json:"current_version"`
}

type brewNames []string

func (names *brewNames) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		if name != "" {
			*names = []string{name}
		}
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*names = values
	return nil
}

func parseBrewJSON(output string, installed, outdated bool) ([]Package, error) {
	var document brewDocument
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		return nil, fmt.Errorf("pkgmgr: parse Homebrew JSON: %w", err)
	}
	packages := make([]Package, 0, len(document.Formulae)+len(document.Casks))
	for _, formula := range document.Formulae {
		name := formula.FullName
		if name == "" {
			name = formula.Name
		}
		item := Package{Name: name, Description: formula.Desc, Installed: installed}
		if len(formula.Installed) > 0 {
			item.Version = formula.Installed[len(formula.Installed)-1].Version
			item.Installed = true
		} else if len(formula.InstalledVersions) > 0 {
			item.Version = formula.InstalledVersions[len(formula.InstalledVersions)-1]
			item.Installed = true
		}
		if outdated {
			item.LatestVersion = formula.CurrentVersion
		} else if item.Version == "" {
			item.Version = formula.Versions.Stable
		}
		packages = append(packages, item)
	}
	for _, cask := range document.Casks {
		name := cask.Token
		if name == "" && len(cask.Name) > 0 {
			name = cask.Name[0]
		}
		if name == "" {
			continue
		}
		item := Package{Name: name, Description: cask.Desc, Installed: installed}
		if cask.Installed != "" {
			item.Version = cask.Installed
			item.Installed = true
		} else if len(cask.InstalledVersions) > 0 {
			item.Version = cask.InstalledVersions[len(cask.InstalledVersions)-1]
			item.Installed = true
		} else {
			item.Version = cask.Version
		}
		if outdated {
			item.LatestVersion = cask.CurrentVersion
		}
		packages = append(packages, item)
	}
	return packages, nil
}

func parseBrewSearch(output string) []Package {
	seen := make(map[string]struct{})
	for _, line := range lines(output) {
		if strings.HasPrefix(line, "==>") {
			continue
		}
		for _, name := range strings.Fields(line) {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	packages := make([]Package, 0, len(names))
	for _, name := range names {
		packages = append(packages, Package{Name: name})
	}
	return packages
}

func parseBrewInstalled(output string) []Package {
	var packages []Package
	for _, line := range lines(output) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		item := Package{Name: fields[0], Installed: true}
		if len(fields) > 1 {
			item.Version = fields[len(fields)-1]
		}
		packages = append(packages, item)
	}
	return packages
}

func lines(output string) []string {
	raw := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(raw))
	for _, line := range raw {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
