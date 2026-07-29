package pkgmgr

type apkAdapter struct {
	binary string
}

func (a apkAdapter) backend() Backend           { return APK }
func (a apkAdapter) displayName() string        { return "APK" }
func (a apkAdapter) executable() string         { return a.binary }
func (a apkAdapter) capabilities() Capabilities { return standardCapabilities(true) }

func (a apkAdapter) changeCommand(operation Operation, packages []string, options ChangeOptions) (Command, error) {
	var args []string
	if options.AssumeYes {
		args = append(args, "--no-interactive")
	}
	switch operation {
	case Refresh:
		args = append(args, "update")
	case Install:
		args = append(args, "add")
	case Remove:
		args = append(args, "del")
	case Upgrade:
		args = append(args, "upgrade")
	case Clean:
		args = append(args, "cache", "clean")
	default:
		return Command{}, ErrUnsupported
	}
	args = append(args, packages...)
	return Command{Name: a.binary, Args: args}, nil
}

func (a apkAdapter) installedCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"list", "--installed"}}, nil
}

func (a apkAdapter) upgradableCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"list", "--upgradeable"}}, nil
}

func (a apkAdapter) searchCommand(query string) (Command, error) {
	return Command{Name: a.binary, Args: []string{"search", "-v", query}}, nil
}

func (a apkAdapter) infoCommand(packages []string) (Command, error) {
	return Command{Name: a.binary, Args: append([]string{"info", "-v"}, packages...)}, nil
}

func (a apkAdapter) parseInstalled(output string) ([]Package, error) {
	return parseAPKList(output, true, false), nil
}

func (a apkAdapter) parseUpgradable(output string) ([]Package, error) {
	return parseAPKList(output, true, true), nil
}

func (a apkAdapter) parseSearch(output string) ([]Package, error) {
	return parseAPKSearch(output), nil
}

func (a apkAdapter) parseInfo(output string) ([]Package, error) {
	return parseAPKVersionLines(output), nil
}
