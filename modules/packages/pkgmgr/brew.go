package pkgmgr

type brewAdapter struct {
	binary string
}

func (a brewAdapter) backend() Backend           { return Brew }
func (a brewAdapter) displayName() string        { return "Homebrew" }
func (a brewAdapter) executable() string         { return a.binary }
func (a brewAdapter) capabilities() Capabilities { return standardCapabilities(true) }

func (a brewAdapter) changeCommand(operation Operation, packages []string, options ChangeOptions) (Command, error) {
	var args []string
	switch operation {
	case Refresh:
		args = []string{"update"}
	case Install:
		args = []string{"install"}
	case Remove:
		args = []string{"uninstall"}
	case Upgrade:
		args = []string{"upgrade"}
	case Clean:
		args = []string{"cleanup"}
	default:
		return Command{}, ErrUnsupported
	}
	if options.AssumeYes && operation == Upgrade {
		args = append(args, "--no-ask")
	}
	args = append(args, packages...)
	return Command{Name: a.binary, Args: args}, nil
}

func (a brewAdapter) installedCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"list"}}, nil
}

func (a brewAdapter) upgradableCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"outdated", "--json=v2"}}, nil
}

func (a brewAdapter) searchCommand(query string) (Command, error) {
	return Command{Name: a.binary, Args: []string{"search", query}}, nil
}

func (a brewAdapter) infoCommand(packages []string) (Command, error) {
	return Command{Name: a.binary, Args: append([]string{"info", "--json=v2"}, packages...)}, nil
}

func (a brewAdapter) parseInstalled(output string) ([]Package, error) {
	return parseBrewInstalled(output), nil
}

func (a brewAdapter) parseUpgradable(output string) ([]Package, error) {
	return parseBrewJSON(output, true, true)
}

func (a brewAdapter) parseSearch(output string) ([]Package, error) {
	return parseBrewSearch(output), nil
}

func (a brewAdapter) parseInfo(output string) ([]Package, error) {
	return parseBrewJSON(output, false, false)
}
