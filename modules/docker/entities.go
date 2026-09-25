// Package docker defines public Docker service and API data contracts.
package docker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
)

// Service is the small Docker capability set used by this module. It does not
// expose the panel's database or any other module's repository.
type Service interface {
	ListDockerContainers(context.Context) ([]types.Container, error)
	InspectDockerContainer(context.Context, string) (types.ContainerJSON, error)
	GetDockerContainerLogs(context.Context, string) (string, error)
	StartDockerContainer(context.Context, string) error
	StopDockerContainer(context.Context, string) error
	RestartDockerContainer(context.Context, string) error
	RemoveDockerContainer(context.Context, string) error
	CreateDockerContainer(context.Context, string, string, string) (string, error)
	ListDockerImages(context.Context) ([]image.Summary, error)
	PullDockerImage(context.Context, string, func(string) error) error
	BuildDockerImage(context.Context, BuildImageRequest, func(string) error) error
	RemoveDockerImage(context.Context, string) error
	ListDockerVolumes(context.Context) (volume.ListResponse, error)
	RemoveDockerVolume(context.Context, string) error
	CreateDockerVolume(context.Context, string, string) error
	ListDockerNetworks(context.Context) ([]types.NetworkResource, error)
	RemoveDockerNetwork(context.Context, string) error
	CreateDockerNetwork(context.Context, string, string, string, string, string) error
	ListDockerComposeProjects(context.Context) ([]ComposeProject, error)
	GetDockerComposeProject(context.Context, string) (*ComposeProjectDetail, error)
	DockerComposeAction(context.Context, string, string) error
	CreateDockerComposeProject(context.Context, ComposeProjectRequest) (*ComposeProject, error)
	GetDockerContainerStats(context.Context, string) (ContainerStats, error)
	ListDockerApps(context.Context, string, string) ([]AppListItem, error)
	GetDockerApp(context.Context, string) (*AppDetail, error)
	ListDockerAppCategories(context.Context) ([]string, error)
	SyncDockerApps(context.Context) error
	GetDockerAppIcon(context.Context, string) ([]byte, string, error)
	StartDockerAppInstall(context.Context, string, map[string]string) error
	GetDockerAppInstall(context.Context, string) (AppInstallProgress, error)
	DockerAppAction(context.Context, string, string) error
}

// AppCatalogStore is the persistence capability needed by LocalService. The
// SQLite implementation lives in the web package so the public client package
// stays independent of database details.
type AppCatalogStore interface {
	ListAppTemplates(context.Context, string) ([]*AppTemplate, error)
	GetAppTemplate(context.Context, string) (*AppTemplate, error)
	AppCategories(context.Context) ([]string, error)
	ReconcileAppCatalog(context.Context, []AppCatalogRecord) error
}

type AppTemplate struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	Available              bool            `json:"available"`
	Exposable              bool            `json:"exposable"`
	DynamicConfig          bool            `json:"dynamic_config"`
	Port                   int             `json:"port"`
	PlatformVersion        int             `json:"platform_version"`
	Version                string          `json:"version"`
	Categories             []string        `json:"categories"`
	Description            string          `json:"description"`
	ShortDesc              string          `json:"short_desc"`
	Author                 string          `json:"author"`
	Source                 string          `json:"source"`
	FormFields             json.RawMessage `json:"form_fields"`
	SupportedArchitectures []string        `json:"supported_architectures"`
	CreatedAt              int64           `json:"created_at"`
	UpdatedAt              int64           `json:"updated_at"`
	ForcePull              bool            `json:"force_pull"`
	MinVersion             string          `json:"min_version"`
	Icon                   string          `json:"icon"`
	ComposeFile            string          `json:"-"`
}

type AppCatalogRecord struct {
	App            AppTemplate
	CategoriesJSON string
	ArchsJSON      string
	FormFieldsJSON string
	ContentHash    string
}

type AppStatus struct {
	State     string `json:"state"`
	Container string `json:"container,omitempty"`
}
type AppListItem struct {
	AppTemplate
	Status AppStatus `json:"status"`
}
type AppDetail struct {
	App     AppTemplate        `json:"app"`
	Status  AppStatus          `json:"status"`
	Install AppInstallProgress `json:"install"`
}
type AppInstallProgress struct {
	Status string   `json:"status"`
	Logs   []string `json:"logs"`
}

type ContainerStats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetworkRx     uint64  `json:"network_rx"`
	NetworkTx     uint64  `json:"network_tx"`
}

type ComposeProjectRequest struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Directory  string `json:"directory,omitempty"`
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Compose    string `json:"compose,omitempty"`
}

type BuildImageRequest struct {
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
	Dockerfile string `json:"dockerfile,omitempty"`
	BuildDir   string `json:"build_dir,omitempty"`
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

type ComposeProject struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	ConfigFiles string    `json:"config_files"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}
type ComposeProjectDetail struct {
	Project    ComposeProject   `json:"project"`
	Containers []map[string]any `json:"containers"`
}
