package pkgmgr

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

// LookPathFunc resolves an executable name.
type LookPathFunc func(string) (string, error)

// Options configures a Manager.
type Options struct {
	Executor         Executor
	MutatingExecutor Executor
	LookPath         LookPathFunc
	Env              []string
}

// Manager provides normalized package operations for one backend.
type Manager struct {
	mu      sync.Mutex
	adapter adapter
	query   Executor
	mutate  Executor
	env     []string
}

// New constructs a manager for an explicitly selected backend.
func New(backend Backend, options Options) (*Manager, error) {
	lookPath := options.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	implementation, err := newAdapter(backend, lookPath)
	if err != nil {
		return nil, err
	}
	query := options.Executor
	if query == nil {
		query = ExecExecutor{}
	}
	mutate := options.MutatingExecutor
	if mutate == nil {
		mutate = query
	}
	env := []string{"LC_ALL=C", "LANG=C"}
	env = append(env, options.Env...)
	return &Manager{
		adapter: implementation,
		query:   query,
		mutate:  mutate,
		env:     env,
	}, nil
}

// Detect constructs the preferred native manager found on the current host.
func Detect(options Options) (*Manager, error) {
	backends, err := DetectAll(options)
	if err != nil {
		return nil, err
	}
	if len(backends) == 0 {
		return nil, ErrNotFound
	}
	return New(backends[0], options)
}

// DetectAll returns all supported package managers found on the current host in
// preferred order.
func DetectAll(options Options) ([]Backend, error) {
	lookPath := options.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	order := []Backend{APT, DNF, Pacman, APK, OPKG, Brew}
	if runtime.GOOS == "darwin" {
		order = []Backend{Brew, APT, DNF, Pacman, APK, OPKG}
	}
	found := make([]Backend, 0, len(order))
	for _, backend := range order {
		if _, err := newAdapter(backend, lookPath); err == nil {
			found = append(found, backend)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	return found, nil
}

func (m *Manager) Backend() Backend {
	return m.adapter.backend()
}

func (m *Manager) Name() string {
	return m.adapter.displayName()
}

// Executable returns the resolved primary package manager executable.
func (m *Manager) Executable() string {
	return m.adapter.executable()
}

func (m *Manager) Capabilities() Capabilities {
	return m.adapter.capabilities()
}

// Command builds but does not execute a mutation command. This is useful for
// previews and audit logs.
func (m *Manager) Command(operation Operation, packages []string, options ChangeOptions) (Command, error) {
	if err := validateOperationPackages(operation, packages); err != nil {
		return Command{}, err
	}
	command, err := m.adapter.changeCommand(operation, packages, options)
	if err != nil {
		return Command{}, err
	}
	command.Env = append(command.Env, m.env...)
	return command, nil
}

// Apply executes a mutation command.
func (m *Manager) Apply(ctx context.Context, operation Operation, packages []string, options ChangeOptions) (Result, error) {
	if m.Backend() == OPKG && operation == Upgrade && len(packages) == 0 {
		return m.upgradeAllOPKG(ctx, options)
	}
	command, err := m.Command(operation, packages, options)
	if err != nil {
		return Result{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mutate.Run(ctx, command)
}

func (m *Manager) Refresh(ctx context.Context, options ChangeOptions) (Result, error) {
	return m.Apply(ctx, Refresh, nil, options)
}

func (m *Manager) Install(ctx context.Context, packages []string, options ChangeOptions) (Result, error) {
	return m.Apply(ctx, Install, packages, options)
}

func (m *Manager) Remove(ctx context.Context, packages []string, options ChangeOptions) (Result, error) {
	return m.Apply(ctx, Remove, packages, options)
}

func (m *Manager) Upgrade(ctx context.Context, packages []string, options ChangeOptions) (Result, error) {
	return m.Apply(ctx, Upgrade, packages, options)
}

func (m *Manager) Clean(ctx context.Context, options ChangeOptions) (Result, error) {
	return m.Apply(ctx, Clean, nil, options)
}

func (m *Manager) ListInstalled(ctx context.Context) ([]Package, error) {
	return m.queryPackages(ctx, m.adapter.installedCommand, m.adapter.parseInstalled)
}

func (m *Manager) ListUpgradable(ctx context.Context) ([]Package, error) {
	return m.queryPackages(ctx, m.adapter.upgradableCommand, m.adapter.parseUpgradable)
}

func (m *Manager) Search(ctx context.Context, query string) ([]Package, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errorsf("search query is empty")
	}
	if strings.HasPrefix(query, "-") || strings.ContainsAny(query, "\x00\r\n") {
		return nil, errorsf("invalid search query %q", query)
	}
	return m.queryPackages(ctx,
		func() (Command, error) { return m.adapter.searchCommand(query) },
		m.adapter.parseSearch,
	)
}

func (m *Manager) Info(ctx context.Context, packages []string) ([]Package, error) {
	if err := validatePackageNames(packages); err != nil {
		return nil, err
	}
	return m.queryPackages(ctx,
		func() (Command, error) { return m.adapter.infoCommand(packages) },
		m.adapter.parseInfo,
	)
}

func (m *Manager) queryPackages(
	ctx context.Context,
	build func() (Command, error),
	parse func(string) ([]Package, error),
) ([]Package, error) {
	command, err := build()
	if err != nil {
		return nil, err
	}
	command.Env = append(command.Env, m.env...)
	m.mu.Lock()
	defer m.mu.Unlock()
	result, err := m.query.Run(ctx, command)
	if err != nil {
		return nil, err
	}
	return parse(result.Stdout)
}

func (m *Manager) upgradeAllOPKG(ctx context.Context, options ChangeOptions) (Result, error) {
	query, err := m.adapter.upgradableCommand()
	if err != nil {
		return Result{}, err
	}
	query.Env = append(query.Env, m.env...)

	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.query.Run(ctx, query)
	if err != nil {
		return Result{}, fmt.Errorf("pkgmgr: resolve opkg upgrades: %w", err)
	}
	upgradable, err := m.adapter.parseUpgradable(result.Stdout)
	if err != nil {
		return Result{}, fmt.Errorf("pkgmgr: parse opkg upgrades: %w", err)
	}
	if len(upgradable) == 0 {
		return Result{ExitCode: 0, Skipped: true}, nil
	}
	packages := make([]string, 0, len(upgradable))
	for _, item := range upgradable {
		packages = append(packages, item.Name)
	}
	if err := validatePackageNames(packages); err != nil {
		return Result{}, err
	}
	command, err := m.adapter.changeCommand(Upgrade, packages, options)
	if err != nil {
		return Result{}, err
	}
	command.Env = append(command.Env, m.env...)
	return m.mutate.Run(ctx, command)
}

func validateOperationPackages(operation Operation, packages []string) error {
	switch operation {
	case Install, Remove:
		if len(packages) == 0 {
			return errorsf("%s requires at least one package", operation)
		}
	case Refresh, Clean:
		if len(packages) != 0 {
			return errorsf("%s does not accept packages", operation)
		}
	case Upgrade:
	default:
		return fmt.Errorf("%w: %q", ErrUnsupported, operation)
	}
	return validatePackageNamesAllowEmpty(packages)
}

func validatePackageNames(packages []string) error {
	if len(packages) == 0 {
		return errorsf("at least one package is required")
	}
	return validatePackageNamesAllowEmpty(packages)
}

func validatePackageNamesAllowEmpty(packages []string) error {
	for _, name := range packages {
		if name == "" || name != strings.TrimSpace(name) || strings.HasPrefix(name, "-") ||
			strings.ContainsAny(name, ";&|`$(){}<>!") {
			return errorsf("invalid package reference %q", name)
		}
		for _, char := range name {
			if unicode.IsSpace(char) || unicode.IsControl(char) {
				return errorsf("invalid package reference %q", name)
			}
		}
	}
	return nil
}

func errorsf(format string, args ...any) error {
	return fmt.Errorf("pkgmgr: "+format, args...)
}
