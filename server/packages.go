package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"os/user"
	"sort"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/pkgmgr"
)

const (
	packageQueryTimeout    = 90 * time.Second
	packageMutationTimeout = 30 * time.Minute
)

type packageManagerRuntime interface {
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

type unavailableMutationExecutor struct {
	message string
}

func (e unavailableMutationExecutor) Run(context.Context, pkgmgr.Command) (pkgmgr.Result, error) {
	return pkgmgr.Result{}, errors.New(e.message)
}

func (s *Server) systemPackageManager() (packageManagerRuntime, error) {
	if s.packageManagerFactory != nil {
		return s.packageManagerFactory()
	}
	detected, err := pkgmgr.Detect(pkgmgr.Options{})
	if err != nil {
		return nil, err
	}
	options := pkgmgr.Options{}
	if detected.Backend() != pkgmgr.Brew && !currentUserIsRoot() {
		options.MutatingExecutor = systemPackageMutationExecutor()
	}
	return pkgmgr.New(detected.Backend(), options)
}

func currentUserIsRoot() bool {
	current, err := user.Current()
	return err == nil && current.Uid == "0"
}

func systemPackageMutationExecutor() pkgmgr.Executor {
	if path, err := exec.LookPath("sudo"); err == nil {
		return pkgmgr.PrefixExecutor{
			Name: path, Args: []string{"-n"}, Inner: pkgmgr.ExecExecutor{},
		}
	}
	if path, err := exec.LookPath("doas"); err == nil {
		return pkgmgr.PrefixExecutor{
			Name: path, Args: []string{"-n"}, Inner: pkgmgr.ExecExecutor{},
		}
	}
	return unavailableMutationExecutor{
		message: "package changes require root or a non-interactive sudo/doas policy",
	}
}

func (s *Server) SystemPackagesView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	manager, err := s.systemPackageManager()
	if err != nil {
		if errors.Is(err, pkgmgr.ErrNotFound) {
			s.Render(w, "packages/index", H{
				"unavailable": true,
				"message":     r.URL.Query().Get("message"),
			})
			return
		}
		s.Error(w, err)
		return
	}

	view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
	if view == "" {
		view = "installed"
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	ctx, cancel := context.WithTimeout(r.Context(), packageQueryTimeout)
	defer cancel()

	var packages []pkgmgr.Package
	switch view {
	case "installed":
		packages, err = manager.ListInstalled(ctx)
	case "upgradable":
		packages, err = manager.ListUpgradable(ctx)
	case "search":
		if query != "" {
			packages, err = manager.Search(ctx, query)
		}
	default:
		http.Error(w, "unknown package view", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.Error(w, fmt.Errorf("%s package query: %w", manager.Name(), err))
		return
	}
	sort.Slice(packages, func(i, j int) bool {
		return strings.ToLower(packages[i].Name) < strings.ToLower(packages[j].Name)
	})

	s.Render(w, "packages/index", H{
		"managerName":  manager.Name(),
		"backend":      manager.Backend(),
		"executable":   manager.Executable(),
		"capabilities": manager.Capabilities(),
		"mutationMode": s.packageMutationMode(manager.Backend()),
		"packages":     packages,
		"view":         view,
		"query":        query,
		"message":      r.URL.Query().Get("message"),
	})
}

func (s *Server) packageMutationMode(backend pkgmgr.Backend) string {
	if s.packageManagerFactory != nil {
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

func (s *Server) SystemPackagesRefreshHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.runPackageMutation(w, r, "refresh", nil)
}

func (s *Server) SystemPackagesInstallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimSpace(r.FormValue("package"))
	if name == "" {
		http.Error(w, "package is required", http.StatusBadRequest)
		return
	}
	s.runPackageMutation(w, r, "install", []string{name})
}

func (s *Server) SystemPackagesRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "package is required", http.StatusBadRequest)
		return
	}
	s.runPackageMutation(w, r, "remove", []string{name})
}

func (s *Server) SystemPackagesUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	var packages []string
	if name != "" {
		packages = []string{name}
	}
	s.runPackageMutation(w, r, "upgrade", packages)
}

func (s *Server) runPackageMutation(w http.ResponseWriter, r *http.Request, operation string, packages []string) {
	manager, err := s.systemPackageManager()
	if err != nil {
		s.Error(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), packageMutationTimeout)
	defer cancel()
	options := pkgmgr.ChangeOptions{AssumeYes: true}

	var result pkgmgr.Result
	switch operation {
	case "refresh":
		result, err = manager.Refresh(ctx, options)
	case "install":
		result, err = manager.Install(ctx, packages, options)
	case "remove":
		result, err = manager.Remove(ctx, packages, options)
	case "upgrade":
		result, err = manager.Upgrade(ctx, packages, options)
	default:
		http.Error(w, "unknown package operation", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("Package operation failed: backend=%s operation=%s packages=%q error=%v",
			manager.Backend(), operation, packages, err)
		s.Error(w, err)
		return
	}
	log.Printf("Package operation completed: backend=%s operation=%s packages=%q exit=%d duration=%s skipped=%t",
		manager.Backend(), operation, packages, result.ExitCode, result.Duration, result.Skipped)

	message := packageMutationMessage(operation, packages, result.Skipped)
	targetView := "installed"
	if operation == "upgrade" || operation == "refresh" {
		targetView = "upgradable"
	}
	target := "/packages?view=" + url.QueryEscape(targetView) + "&message=" + url.QueryEscape(message)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func packageMutationMessage(operation string, packages []string, skipped bool) string {
	if skipped {
		return "All packages are already up to date."
	}
	if len(packages) == 0 {
		switch operation {
		case "refresh":
			return "Package metadata refreshed."
		case "upgrade":
			return "Package upgrade completed."
		}
	}
	return fmt.Sprintf("Package %s completed: %s.", operation, strings.Join(packages, ", "))
}
