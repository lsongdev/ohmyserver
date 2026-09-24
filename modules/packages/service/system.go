package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"os/user"
	"sort"
	"strings"
	"time"

	packages "github.com/lsongdev/ohmyserver/modules/packages"
	"github.com/lsongdev/ohmyserver/modules/packages/pkgmgr"
)

const queryTimeout = 90 * time.Second
const mutationTimeout = 30 * time.Minute

type PackageManager interface {
	Backend() pkgmgr.Backend
	Name() string
	Executable() string
	Capabilities() pkgmgr.Capabilities
	ListInstalled(context.Context) ([]pkgmgr.Package, error)
	ListUpgradable(context.Context) ([]pkgmgr.Package, error)
	Search(context.Context, string) ([]pkgmgr.Package, error)
	Refresh(context.Context, pkgmgr.ChangeOptions) (pkgmgr.Result, error)
	Install(context.Context, []string, pkgmgr.ChangeOptions) (pkgmgr.Result, error)
	Remove(context.Context, []string, pkgmgr.ChangeOptions) (pkgmgr.Result, error)
	Upgrade(context.Context, []string, pkgmgr.ChangeOptions) (pkgmgr.Result, error)
}

type ManagerFactory func() (PackageManager, error)

type SystemService struct {
	managerFactory ManagerFactory
	configured     bool
}

func NewSystemService() *SystemService {
	return &SystemService{managerFactory: detectSystemPackageManager}
}

func NewSystemServiceWithManagerFactory(factory ManagerFactory) *SystemService {
	return &SystemService{managerFactory: factory, configured: true}
}

func (s *SystemService) PackageCatalog(ctx context.Context, view, query string) (packages.Catalog, error) {
	view, query = strings.ToLower(strings.TrimSpace(view)), strings.TrimSpace(query)
	if view != "installed" && view != "upgradable" && view != "search" {
		return packages.Catalog{}, errors.Join(packages.ErrInvalidRequest, fmt.Errorf("unknown package view %q", view))
	}
	manager, err := s.managerFactory()
	if err != nil {
		if errors.Is(err, pkgmgr.ErrNotFound) {
			return packages.Catalog{}, errors.Join(packages.ErrUnavailable, err)
		}
		return packages.Catalog{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var items []pkgmgr.Package
	switch view {
	case "installed":
		items, err = manager.ListInstalled(ctx)
	case "upgradable":
		items, err = manager.ListUpgradable(ctx)
	case "search":
		if query != "" {
			items, err = manager.Search(ctx, query)
		}
	}
	if err != nil {
		return packages.Catalog{}, fmt.Errorf("%s package query: %w", manager.Name(), err)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	result := packages.Catalog{Manager: manager.Name(), Backend: string(manager.Backend()), Executable: manager.Executable(), MutationMode: mutationMode(manager.Backend(), s.configured), Packages: make([]packages.Package, 0, len(items))}
	caps := manager.Capabilities()
	result.Capabilities = packages.Capabilities{Search: caps.Search, Info: caps.Info, ListInstalled: caps.ListInstalled, ListUpgradable: caps.ListUpgradable, Refresh: caps.Refresh, Install: caps.Install, Remove: caps.Remove, Upgrade: caps.Upgrade, Clean: caps.Clean, SafeUpgradeAll: caps.SafeUpgradeAll}
	for _, item := range items {
		result.Packages = append(result.Packages, packages.Package{Name: item.Name, Version: item.Version, LatestVersion: item.LatestVersion, Description: item.Description, Architecture: item.Architecture, Repository: item.Repository, Installed: item.Installed})
	}
	return result, nil
}

func (s *SystemService) ChangePackages(ctx context.Context, operation, name string) (packages.ChangeResult, error) {
	if operation != "refresh" && operation != "install" && operation != "remove" && operation != "upgrade" {
		return packages.ChangeResult{}, errors.Join(packages.ErrInvalidRequest, fmt.Errorf("unknown package operation %q", operation))
	}
	if (operation == "install" || operation == "remove") && name == "" {
		return packages.ChangeResult{}, errors.Join(packages.ErrInvalidRequest, errors.New("package name is required"))
	}
	manager, err := s.managerFactory()
	if err != nil {
		if errors.Is(err, pkgmgr.ErrNotFound) {
			return packages.ChangeResult{}, errors.Join(packages.ErrUnavailable, err)
		}
		return packages.ChangeResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, mutationTimeout)
	defer cancel()
	options := pkgmgr.ChangeOptions{AssumeYes: true}
	var result pkgmgr.Result
	var names []string
	if name != "" {
		names = []string{name}
	}
	switch operation {
	case "refresh":
		result, err = manager.Refresh(ctx, options)
	case "install":
		result, err = manager.Install(ctx, names, options)
	case "remove":
		result, err = manager.Remove(ctx, names, options)
	case "upgrade":
		result, err = manager.Upgrade(ctx, names, options)
	}
	if err != nil {
		log.Printf("Package operation failed: backend=%s operation=%s packages=%q error=%v", manager.Backend(), operation, names, err)
		return packages.ChangeResult{}, err
	}
	log.Printf("Package operation completed: backend=%s operation=%s packages=%q exit=%d duration=%s skipped=%t", manager.Backend(), operation, names, result.ExitCode, result.Duration, result.Skipped)
	return packages.ChangeResult{Operation: operation, Packages: names, Skipped: result.Skipped, ExitCode: result.ExitCode, Duration: result.Duration.Round(time.Millisecond).String(), Output: strings.TrimSpace(result.Stdout + "\n" + result.Stderr)}, nil
}

func detectSystemPackageManager() (PackageManager, error) {
	detected, err := pkgmgr.Detect(pkgmgr.Options{})
	if err != nil {
		return nil, err
	}
	options := pkgmgr.Options{}
	if detected.Backend() != pkgmgr.Brew && !currentUserIsRoot() {
		options.MutatingExecutor = systemMutationExecutor()
	}
	return pkgmgr.New(detected.Backend(), options)
}

func currentUserIsRoot() bool {
	current, err := user.Current()
	return err == nil && current.Uid == "0"
}

type unavailableExecutor struct{ message string }

func (e unavailableExecutor) Run(context.Context, pkgmgr.Command) (pkgmgr.Result, error) {
	return pkgmgr.Result{}, errors.New(e.message)
}

func systemMutationExecutor() pkgmgr.Executor {
	if path, err := exec.LookPath("sudo"); err == nil {
		return pkgmgr.PrefixExecutor{Name: path, Args: []string{"-n"}, Inner: pkgmgr.ExecExecutor{}}
	}
	if path, err := exec.LookPath("doas"); err == nil {
		return pkgmgr.PrefixExecutor{Name: path, Args: []string{"-n"}, Inner: pkgmgr.ExecExecutor{}}
	}
	return unavailableExecutor{message: "package changes require root or a non-interactive sudo/doas policy"}
}

func mutationMode(backend pkgmgr.Backend, configured bool) string {
	if configured {
		return "configured executor"
	}
	if backend == pkgmgr.Brew {
		return "current user"
	}
	if currentUserIsRoot() {
		return "direct root execution"
	}
	if path, err := exec.LookPath("sudo"); err == nil {
		return path + " -n (preconfigured policy required)"
	}
	if path, err := exec.LookPath("doas"); err == nil {
		return path + " -n (preconfigured policy required)"
	}
	return "unavailable: root or a sudo/doas policy is required"
}
