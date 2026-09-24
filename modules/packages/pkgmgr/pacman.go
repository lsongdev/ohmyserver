package pkgmgr

type pacmanAdapter struct {
	binary string
}

func (a pacmanAdapter) backend() Backend           { return Pacman }
func (a pacmanAdapter) displayName() string        { return "Pacman" }
func (a pacmanAdapter) executable() string         { return a.binary }
func (a pacmanAdapter) capabilities() Capabilities { return standardCapabilities(true) }

func (a pacmanAdapter) changeCommand(operation Operation, packages []string, options ChangeOptions) (Command, error) {
	var args []string
	switch operation {
	case Refresh:
		args = []string{"-Sy"}
	case Install:
		args = []string{"-S", "--needed"}
	case Remove:
		args = []string{"-R"}
	case Upgrade:
		if len(packages) == 0 {
			args = []string{"-Syu"}
		} else {
			args = []string{"-S"}
		}
	case Clean:
		args = []string{"-Sc"}
	default:
		return Command{}, ErrUnsupported
	}
	if options.AssumeYes {
		args = append(args, "--noconfirm")
	}
	args = append(args, packages...)
	return Command{Name: a.binary, Args: args}, nil
}

func (a pacmanAdapter) installedCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"-Q"}}, nil
}

func (a pacmanAdapter) upgradableCommand() (Command, error) {
	return Command{Name: a.binary, Args: []string{"-Qu"}}, nil
}

func (a pacmanAdapter) searchCommand(query string) (Command, error) {
	return Command{Name: a.binary, Args: []string{"-Ss", query}}, nil
}

func (a pacmanAdapter) infoCommand(packages []string) (Command, error) {
	return Command{Name: a.binary, Args: append([]string{"-Si"}, packages...)}, nil
}

func (a pacmanAdapter) parseInstalled(output string) ([]Package, error) {
	return parseSpacePackages(output, true), nil
}

func (a pacmanAdapter) parseUpgradable(output string) ([]Package, error) {
	return parsePacmanUpgradable(output), nil
}

func (a pacmanAdapter) parseSearch(output string) ([]Package, error) {
	return parsePacmanSearch(output), nil
}

func (a pacmanAdapter) parseInfo(output string) ([]Package, error) {
	return parseColonParagraphs(output), nil
}
