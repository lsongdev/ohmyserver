package pkgmgr

import (
	"fmt"
)

type adapter interface {
	backend() Backend
	displayName() string
	executable() string
	capabilities() Capabilities
	changeCommand(Operation, []string, ChangeOptions) (Command, error)
	installedCommand() (Command, error)
	upgradableCommand() (Command, error)
	searchCommand(string) (Command, error)
	infoCommand([]string) (Command, error)
	parseInstalled(string) ([]Package, error)
	parseUpgradable(string) ([]Package, error)
	parseSearch(string) ([]Package, error)
	parseInfo(string) ([]Package, error)
}

type paths map[string]string

func newAdapter(backend Backend, lookPath LookPathFunc) (adapter, error) {
	required, err := requiredExecutables(backend)
	if err != nil {
		return nil, err
	}
	resolved := make(paths, len(required))
	for role, candidates := range required {
		var path string
		for _, candidate := range candidates {
			if found, err := lookPath(candidate); err == nil {
				path = found
				break
			}
		}
		if path == "" {
			return nil, fmt.Errorf("%w: %s requires %s", ErrNotFound, backend, candidates)
		}
		resolved[role] = path
	}
	switch backend {
	case APT:
		return aptAdapter{paths: resolved}, nil
	case DNF:
		return dnfAdapter{binary: resolved["main"]}, nil
	case Pacman:
		return pacmanAdapter{binary: resolved["main"]}, nil
	case APK:
		return apkAdapter{binary: resolved["main"]}, nil
	case OPKG:
		return opkgAdapter{binary: resolved["main"]}, nil
	case Brew:
		return brewAdapter{binary: resolved["main"]}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedBackend, backend)
	}
}

func primaryExecutables(backend Backend) ([]string, error) {
	switch backend {
	case APT:
		return []string{"apt-get"}, nil
	case DNF:
		return []string{"dnf5", "dnf"}, nil
	case Pacman:
		return []string{"pacman"}, nil
	case APK:
		return []string{"apk"}, nil
	case OPKG:
		return []string{"opkg"}, nil
	case Brew:
		return []string{"brew"}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedBackend, backend)
	}
}

func requiredExecutables(backend Backend) (map[string][]string, error) {
	main, err := primaryExecutables(backend)
	if err != nil {
		return nil, err
	}
	required := map[string][]string{"main": main}
	if backend == APT {
		required["cache"] = []string{"apt-cache"}
		required["list"] = []string{"apt"}
		required["query"] = []string{"dpkg-query"}
	}
	return required, nil
}

func standardCapabilities(safeUpgradeAll bool) Capabilities {
	return Capabilities{
		Search:         true,
		Info:           true,
		ListInstalled:  true,
		ListUpgradable: true,
		Refresh:        true,
		Install:        true,
		Remove:         true,
		Upgrade:        true,
		Clean:          true,
		SafeUpgradeAll: safeUpgradeAll,
	}
}
