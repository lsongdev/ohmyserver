package pkgmgr

type dnfAdapter struct {
	binary string
}

func (a dnfAdapter) backend() Backend           { return DNF }
func (a dnfAdapter) displayName() string        { return "DNF" }
func (a dnfAdapter) executable() string         { return a.binary }
func (a dnfAdapter) capabilities() Capabilities { return standardCapabilities(true) }

func (a dnfAdapter) changeCommand(operation Operation, packages []string, options ChangeOptions) (Command, error) {
	args := make([]string, 0, len(packages)+2)
	switch operation {
	case Refresh:
		args = append(args, "makecache")
	case Install:
		args = append(args, "install")
	case Remove:
		args = append(args, "remove")
	case Upgrade:
		args = append(args, "upgrade")
	case Clean:
		args = append(args, "clean", "all")
	default:
		return Command{}, ErrUnsupported
	}
	if options.AssumeYes && operation != Refresh && operation != Clean {
		args = append(args, "--assumeyes")
	}
	args = append(args, packages...)
	return Command{Name: a.binary, Args: args}, nil
}

func (a dnfAdapter) queryCommand(args ...string) Command {
	base := []string{"--quiet", "repoquery", "--queryformat",
		"%{name}\t%{evr}\t%{arch}\t%{summary}"}
	return Command{Name: a.binary, Args: append(base, args...)}
}

func (a dnfAdapter) installedCommand() (Command, error) {
	return a.queryCommand("--installed"), nil
}

func (a dnfAdapter) upgradableCommand() (Command, error) {
	return a.queryCommand("--upgrades"), nil
}

func (a dnfAdapter) searchCommand(query string) (Command, error) {
	return a.queryCommand("--available", "--latest-limit", "1", "*"+query+"*"), nil
}

func (a dnfAdapter) infoCommand(packages []string) (Command, error) {
	args := append([]string{"--latest-limit", "1"}, packages...)
	return a.queryCommand(args...), nil
}

func (a dnfAdapter) parseInstalled(output string) ([]Package, error) {
	return parseTabPackages(output, true, false)
}

func (a dnfAdapter) parseUpgradable(output string) ([]Package, error) {
	return parseTabPackages(output, true, true)
}

func (a dnfAdapter) parseSearch(output string) ([]Package, error) {
	return parseTabPackages(output, false, false)
}

func (a dnfAdapter) parseInfo(output string) ([]Package, error) {
	return parseTabPackages(output, false, false)
}
