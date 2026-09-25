package pkgmgr

type aptAdapter struct {
	paths paths
}

func (a aptAdapter) backend() Backend           { return APT }
func (a aptAdapter) displayName() string        { return "APT" }
func (a aptAdapter) executable() string         { return a.paths["main"] }
func (a aptAdapter) capabilities() Capabilities { return standardCapabilities(true) }

func (a aptAdapter) changeCommand(operation Operation, packages []string, options ChangeOptions) (Command, error) {
	args := make([]string, 0, len(packages)+3)
	switch operation {
	case Refresh:
		args = append(args, "update")
	case Install:
		args = append(args, "install")
	case Remove:
		args = append(args, "remove")
	case Upgrade:
		if len(packages) == 0 {
			args = append(args, "upgrade")
		} else {
			args = append(args, "install", "--only-upgrade")
		}
	case Clean:
		args = append(args, "clean")
	default:
		return Command{}, ErrUnsupported
	}
	if options.AssumeYes && operation != Refresh && operation != Clean {
		args = append(args, "--yes")
	}
	args = append(args, packages...)
	return Command{Name: a.paths["main"], Args: args}, nil
}

func (a aptAdapter) installedCommand() (Command, error) {
	return Command{
		Name: a.paths["query"],
		Args: []string{"-W", "-f=${binary:Package}\t${Version}\t${Architecture}\t${binary:Summary}\n"},
	}, nil
}

func (a aptAdapter) upgradableCommand() (Command, error) {
	return Command{Name: a.paths["list"], Args: []string{"list", "--upgradable"}}, nil
}

func (a aptAdapter) searchCommand(query string) (Command, error) {
	return Command{Name: a.paths["cache"], Args: []string{"search", "--names-only", query}}, nil
}

func (a aptAdapter) infoCommand(packages []string) (Command, error) {
	args := append([]string{"show", "--no-all-versions"}, packages...)
	return Command{Name: a.paths["cache"], Args: args}, nil
}

func (a aptAdapter) parseInstalled(output string) ([]Package, error) {
	return parseTabPackages(output, true, false)
}

func (a aptAdapter) parseUpgradable(output string) ([]Package, error) {
	return parseAPTUpgradable(output), nil
}

func (a aptAdapter) parseSearch(output string) ([]Package, error) {
	return parseDashSearch(output), nil
}

func (a aptAdapter) parseInfo(output string) ([]Package, error) {
	return parseControlParagraphs(output, controlMapping{
		name: "Package", version: "Version", description: "Description",
		architecture: "Architecture",
	}), nil
}
