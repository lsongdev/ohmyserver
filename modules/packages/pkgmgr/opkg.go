package pkgmgr

type opkgAdapter struct {
	binary string
}

func (a opkgAdapter) backend() Backend    { return OPKG }
func (a opkgAdapter) displayName() string { return "opkg" }
func (a opkgAdapter) executable() string  { return a.binary }
func (a opkgAdapter) capabilities() Capabilities {
	capabilities := standardCapabilities(false)
	capabilities.Clean = false
	return capabilities
}

func (a opkgAdapter) changeCommand(operation Operation, packages []string, _ ChangeOptions) (Command, error) {
	var args []string
	switch operation {
	case Refresh:
		args = []string{"update"}
	case Install:
		args = []string{"install"}
	case Remove:
		args = []string{"remove"}
	case Upgrade:
		if len(packages) == 0 {
			return Command{}, errorsf("%w: resolve opkg upgrades before building the command", ErrUnsafeOperation)
		}
		args = []string{"upgrade"}
	case Clean:
		return Command{}, ErrUnsupported
	default:
		return Command{}, ErrUnsupported
	}
	args = append(args, packages...)
	return Command{Name: a.binary, Args: args}, nil
}

func (a opkgAdapter) installedCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"list-installed"}}, nil
}

func (a opkgAdapter) upgradableCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"list-upgradable"}}, nil
}

func (a opkgAdapter) searchCommand(query string) (Command, error) {
	return Command{Name: a.binary, Args: []string{"list", "*" + query + "*"}}, nil
}

func (a opkgAdapter) infoCommand(packages []string) (Command, error) {
	return Command{Name: a.binary, Args: append([]string{"info"}, packages...)}, nil
}

func (a opkgAdapter) parseInstalled(output string) ([]Package, error) {
	return parseOPKGList(output, true, false), nil
}

func (a opkgAdapter) parseUpgradable(output string) ([]Package, error) {
	return parseOPKGList(output, true, true), nil
}

func (a opkgAdapter) parseSearch(output string) ([]Package, error) {
	return parseOPKGList(output, false, false), nil
}

func (a opkgAdapter) parseInfo(output string) ([]Package, error) {
	return parseControlParagraphs(output, controlMapping{
		name: "Package", version: "Version", description: "Description",
		architecture: "Architecture", repository: "Source",
	}), nil
}
