package packages

import (
	"context"
	"errors"
)

var ErrInvalidRequest = errors.New("invalid package request")
var ErrUnavailable = errors.New("no supported package manager available")

type Package struct {
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	LatestVersion string `json:"latest_version,omitempty"`
	Description   string `json:"description,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
	Repository    string `json:"repository,omitempty"`
	Installed     bool   `json:"installed"`
}
type Capabilities struct {
	Search         bool `json:"search"`
	Info           bool `json:"info"`
	ListInstalled  bool `json:"list_installed"`
	ListUpgradable bool `json:"list_upgradable"`
	Refresh        bool `json:"refresh"`
	Install        bool `json:"install"`
	Remove         bool `json:"remove"`
	Upgrade        bool `json:"upgrade"`
	Clean          bool `json:"clean"`
	SafeUpgradeAll bool `json:"safe_upgrade_all"`
}
type Catalog struct {
	Manager      string       `json:"manager"`
	Backend      string       `json:"backend"`
	Executable   string       `json:"executable"`
	MutationMode string       `json:"mutation_mode"`
	Capabilities Capabilities `json:"capabilities"`
	Packages     []Package    `json:"packages"`
}
type ChangeResult struct {
	Operation string   `json:"operation"`
	Packages  []string `json:"packages,omitempty"`
	Skipped   bool     `json:"skipped,omitempty"`
	ExitCode  int      `json:"exit_code"`
	Duration  string   `json:"duration"`
	Output    string   `json:"output,omitempty"`
}
type Service interface {
	PackageCatalog(context.Context, string, string) (Catalog, error)
	ChangePackages(context.Context, string, string) (ChangeResult, error)
}
