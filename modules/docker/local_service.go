package docker

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
)

const defaultAppsRepositoryURL = "https://github.com/lsongdev/ohmyserver-apps.git"

type installTask struct {
	mu     sync.RWMutex
	status string
	logs   []string
}
type LocalService struct {
	ctx        context.Context
	client     *DockerClient
	sdk        *SDKService
	compose    *ComposeService
	repo       AppCatalogStore
	appsDir    string
	composeDir string
	appsURL    string
	appsMu     sync.Mutex
	tasksMu    sync.RWMutex
	tasks      map[string]*installTask
}

func NewLocalService(ctx context.Context, client *DockerClient, repo AppCatalogStore, appsDir, composeDir string) *LocalService {
	return &LocalService{ctx: ctx, client: client, sdk: NewSDKService(client), compose: NewComposeService(composeDir), repo: repo, appsDir: appsDir, composeDir: composeDir, appsURL: defaultAppsRepositoryURL, tasks: make(map[string]*installTask)}
}
func (s *LocalService) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}
func (s *LocalService) ListDockerContainers(ctx context.Context) ([]types.Container, error) {
	return s.sdk.ListDockerContainers(ctx)
}
func (s *LocalService) InspectDockerContainer(ctx context.Context, id string) (types.ContainerJSON, error) {
	return s.sdk.InspectDockerContainer(ctx, id)
}
func (s *LocalService) GetDockerContainerLogs(ctx context.Context, id string) (string, error) {
	return s.sdk.GetDockerContainerLogs(ctx, id)
}
func (s *LocalService) StartDockerContainer(ctx context.Context, id string) error {
	return s.sdk.StartDockerContainer(ctx, id)
}
func (s *LocalService) StopDockerContainer(ctx context.Context, id string) error {
	return s.sdk.StopDockerContainer(ctx, id)
}
func (s *LocalService) RestartDockerContainer(ctx context.Context, id string) error {
	return s.sdk.RestartDockerContainer(ctx, id)
}
func (s *LocalService) RemoveDockerContainer(ctx context.Context, id string) error {
	return s.sdk.RemoveDockerContainer(ctx, id)
}
func (s *LocalService) CreateDockerContainer(ctx context.Context, name, image, command string) (string, error) {
	return s.sdk.CreateDockerContainer(ctx, name, image, command)
}
func (s *LocalService) CreateContainer(ctx context.Context, image, name string, command []string) (string, error) {
	return s.sdk.CreateContainer(ctx, image, name, command)
}
func (s *LocalService) ListDockerImages(ctx context.Context) ([]image.Summary, error) {
	return s.sdk.ListDockerImages(ctx)
}
func (s *LocalService) PullDockerImage(ctx context.Context, name string, progress func(string) error) error {
	return s.sdk.PullDockerImage(ctx, name, progress)
}
func (s *LocalService) BuildDockerImage(ctx context.Context, req BuildImageRequest, log func(string) error) error {
	return s.buildImage(ctx, req, log)
}
func (s *LocalService) RemoveDockerImage(ctx context.Context, id string) error {
	return s.sdk.RemoveDockerImage(ctx, id)
}
func (s *LocalService) ListDockerVolumes(ctx context.Context) (volume.ListResponse, error) {
	return s.sdk.ListDockerVolumes(ctx)
}
func (s *LocalService) RemoveDockerVolume(ctx context.Context, name string) error {
	return s.sdk.RemoveDockerVolume(ctx, name)
}
func (s *LocalService) CreateDockerVolume(ctx context.Context, name, driver string) error {
	return s.sdk.CreateDockerVolume(ctx, name, driver)
}
func (s *LocalService) ListDockerNetworks(ctx context.Context) ([]types.NetworkResource, error) {
	return s.sdk.ListDockerNetworks(ctx)
}
func (s *LocalService) RemoveDockerNetwork(ctx context.Context, id string) error {
	return s.sdk.RemoveDockerNetwork(ctx, id)
}
func (s *LocalService) CreateDockerNetwork(ctx context.Context, name, driver, subnet, gateway, ipRange string) error {
	return s.sdk.CreateDockerNetwork(ctx, name, driver, subnet, gateway, ipRange)
}
func (s *LocalService) ListDockerComposeProjects(ctx context.Context) ([]ComposeProject, error) {
	return s.compose.ListDockerComposeProjects(ctx)
}
func (s *LocalService) GetDockerComposeProject(ctx context.Context, name string) (*ComposeProjectDetail, error) {
	return s.compose.GetDockerComposeProject(ctx, name)
}
func (s *LocalService) DockerComposeAction(ctx context.Context, name, action string) error {
	return s.compose.DockerComposeAction(ctx, name, action)
}
func (s *LocalService) CreateDockerComposeProject(ctx context.Context, input ComposeProjectRequest) (*ComposeProject, error) {
	return s.compose.CreateDockerComposeProject(ctx, input)
}
func (s *LocalService) GetDockerContainerStats(ctx context.Context, id string) (ContainerStats, error) {
	return s.containerStats(ctx, id)
}

func (s *LocalService) buildImage(ctx context.Context, req BuildImageRequest, onLog func(string) error) error {
	var dir string
	cleanup := func() {}
	switch req.SourceType {
	case "dockerfile":
		tmp, err := os.MkdirTemp("", "ohmyserver-dockerfile-build-")
		if err != nil {
			return err
		}
		dir = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(req.Dockerfile), 0600); err != nil {
			cleanup()
			return err
		}
	case "directory":
		dir = filepath.Clean(strings.TrimSpace(req.BuildDir))
	case "repository":
		tmp, err := os.MkdirTemp("", "ohmyserver-docker-repo-build-")
		if err != nil {
			return err
		}
		dir = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
		args := []string{"clone", "--depth=1"}
		if branch := strings.TrimSpace(req.Branch); branch != "" {
			args = append(args, "--branch", branch)
		}
		args = append(args, strings.TrimSpace(req.Repository), dir)
		output, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
		if onLog != nil && len(output) > 0 {
			if logErr := onLog(strings.TrimSpace(string(output))); logErr != nil {
				cleanup()
				return logErr
			}
		}
		if err != nil {
			cleanup()
			return fmt.Errorf("clone build repository: %w", err)
		}
	default:
		return fmt.Errorf("unsupported Docker build source %q", req.SourceType)
	}
	defer cleanup()
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("inspect Docker build directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("Docker build source must be a directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
		return fmt.Errorf("Dockerfile not found in build context: %w", err)
	}
	archive, err := tarDirectory(dir)
	if err != nil {
		return err
	}
	defer archive.Close()
	response, err := s.client.Client.ImageBuild(ctx, archive, types.ImageBuildOptions{Tags: []string{strings.TrimSpace(req.Name)}, Dockerfile: "Dockerfile", Remove: true})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(response.Body)
	for {
		var event struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
			Detail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if event.Stream != "" && onLog != nil {
			if err := onLog(strings.TrimSpace(event.Stream)); err != nil {
				return err
			}
		}
		message := event.Error
		if message == "" {
			message = event.Detail.Message
		}
		if message != "" {
			return errors.New(message)
		}
	}
}
func tarDirectory(root string) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		tw := tar.NewWriter(writer)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path == root {
				return nil
			}
			if info.IsDir() && info.Name() == ".git" {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			link := ""
			if info.Mode()&os.ModeSymlink != 0 {
				link, err = os.Readlink(path)
				if err != nil {
					return err
				}
			}
			header, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(rel)
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				_, copyErr := io.Copy(tw, f)
				closeErr := f.Close()
				if copyErr != nil {
					return copyErr
				}
				if closeErr != nil {
					return closeErr
				}
			}
			return nil
		})
		closeErr := tw.Close()
		if err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	return reader, nil
}
func (s *LocalService) containerStats(ctx context.Context, id string) (ContainerStats, error) {
	response, err := s.client.Client.ContainerStats(ctx, id, false)
	if err != nil {
		return ContainerStats{}, err
	}
	defer response.Body.Close()
	var stats types.StatsJSON
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		return ContainerStats{}, err
	}
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
	cpuPercent := float64(0)
	if systemDelta > 0 && cpuDelta > 0 {
		cpuCount := float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		if cpuCount == 0 {
			cpuCount = 1
		}
		cpuPercent = cpuDelta / systemDelta * cpuCount * 100
	}
	memoryPercent := float64(0)
	if stats.MemoryStats.Limit > 0 {
		memoryPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100
	}
	var rx, tx uint64
	for _, network := range stats.Networks {
		rx += network.RxBytes
		tx += network.TxBytes
	}
	return ContainerStats{CPUPercent: cpuPercent, MemoryUsage: stats.MemoryStats.Usage, MemoryLimit: stats.MemoryStats.Limit, MemoryPercent: memoryPercent, NetworkRx: rx, NetworkTx: tx}, nil
}

func (s *LocalService) ListDockerApps(ctx context.Context, filter, category string) ([]AppListItem, error) {
	apps, err := s.repo.ListAppTemplates(ctx, category)
	if err != nil {
		return nil, err
	}
	statuses, err := s.appStatuses(ctx, apps)
	if err != nil {
		return nil, err
	}
	result := make([]AppListItem, 0, len(apps))
	for _, app := range apps {
		status := statuses[app.ID]
		if filter == "running" && status.State != "running" {
			continue
		}
		if filter == "installed" && status.State != "running" && status.State != "installed" {
			continue
		}
		result = append(result, AppListItem{AppTemplate: *app, Status: status})
	}
	return result, nil
}
func (s *LocalService) appStatuses(ctx context.Context, apps []*AppTemplate) (map[string]AppStatus, error) {
	items, err := s.sdk.ListDockerContainers(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]AppStatus, len(apps))
	for _, app := range apps {
		state := AppStatus{State: "not_installed"}
		if task := s.task(app.ID); task != nil {
			task.mu.RLock()
			if task.status == "installing" {
				state.State = "installing"
			}
			task.mu.RUnlock()
		}
		for _, container := range items {
			if container.Labels["com.docker.compose.project"] == app.ID {
				state.Container = container.ID
				if container.State == "running" {
					state.State = "running"
				} else if state.State != "installing" {
					state.State = "installed"
				}
			}
		}
		result[app.ID] = state
	}
	return result, nil
}
func (s *LocalService) GetDockerApp(ctx context.Context, id string) (*AppDetail, error) {
	app, err := s.repo.GetAppTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	statuses, err := s.appStatuses(ctx, []*AppTemplate{app})
	if err != nil {
		return nil, err
	}
	return &AppDetail{App: *app, Status: statuses[id], Install: s.installProgress(id)}, nil
}
func (s *LocalService) ListDockerAppCategories(ctx context.Context) ([]string, error) {
	return s.repo.AppCategories(ctx)
}
func (s *LocalService) SyncDockerApps(ctx context.Context) error {
	s.appsMu.Lock()
	defer s.appsMu.Unlock()
	if err := syncAppsRepository(ctx, s.appsURL, s.appsDir); err != nil {
		return err
	}
	return s.loadDockerApps(ctx)
}

// LoadDockerApps reconciles the checked-out catalog without contacting its
// remote repository. Hosts use it to serve a cached catalog after sync fails.
func (s *LocalService) LoadDockerApps(ctx context.Context) error {
	s.appsMu.Lock()
	defer s.appsMu.Unlock()
	return s.loadDockerApps(ctx)
}

func (s *LocalService) loadDockerApps(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.appsDir)
	if err != nil {
		return err
	}
	records := make([]AppCatalogRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".git" {
			continue
		}
		dir := filepath.Join(s.appsDir, entry.Name())
		manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			continue
		}
		var app AppTemplate
		if json.Unmarshal(manifest, &app) != nil {
			continue
		}
		app.ID = entry.Name()
		compose, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
		if err != nil {
			continue
		}
		app.ComposeFile = string(compose)
		categories, _ := json.Marshal(app.Categories)
		archs, _ := json.Marshal(app.SupportedArchitectures)
		fields, _ := json.Marshal(app.FormFields)
		raw := append(append(append([]byte(nil), manifest...), 0), compose...)
		records = append(records, AppCatalogRecord{App: app, CategoriesJSON: string(categories), ArchsJSON: string(archs), FormFieldsJSON: string(fields), ContentHash: fmt.Sprintf("%x", sha256.Sum256(raw))})
	}
	if len(records) == 0 {
		return fmt.Errorf("no valid app templates found in %s", s.appsDir)
	}
	return s.repo.ReconcileAppCatalog(ctx, records)
}
func syncAppsRepository(ctx context.Context, url, dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
			return err
		}
		output, err := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, dir).CombinedOutput()
		if err != nil {
			return fmt.Errorf("git clone: %s: %w", strings.TrimSpace(string(output)), err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("apps path is not a directory: %s", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return fmt.Errorf("apps directory is not a git checkout: %s", dir)
	}
	output, err := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
func (s *LocalService) GetDockerAppIcon(ctx context.Context, id string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return nil, "", errors.New("invalid app id")
	}
	if _, err := s.repo.GetAppTemplate(ctx, id); err != nil {
		return nil, "", err
	}
	entries, err := os.ReadDir(filepath.Join(s.appsDir, id))
	if err != nil {
		return nil, "", err
	}
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || !(ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".svg") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.appsDir, id, entry.Name()))
		if err != nil {
			return nil, "", err
		}
		content := "image/jpeg"
		switch ext {
		case ".png":
			content = "image/png"
		case ".svg":
			content = "image/svg+xml"
		}
		return data, content, nil
	}
	return nil, "", os.ErrNotExist
}
func (s *LocalService) StartDockerAppInstall(ctx context.Context, id string, env map[string]string) error {
	app, err := s.repo.GetAppTemplate(ctx, id)
	if err != nil {
		return err
	}
	task := s.task(id)
	task.mu.Lock()
	if task.status == "installing" {
		task.mu.Unlock()
		return errors.New("app installation is already running")
	}
	task.status = "installing"
	task.logs = []string{}
	task.mu.Unlock()
	go s.runInstall(app, env)
	return nil
}
func (s *LocalService) runInstall(app *AppTemplate, envVars map[string]string) {
	task := s.task(app.ID)
	logLine := func(v string) { task.mu.Lock(); task.logs = append(task.logs, v); task.mu.Unlock() }
	appDir := filepath.Join(s.composeDir, app.ID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		logLine(err.Error())
		task.mu.Lock()
		task.status = "failed"
		task.mu.Unlock()
		return
	}
	composeFile := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte(app.ComposeFile), 0644); err != nil {
		logLine(err.Error())
		task.mu.Lock()
		task.status = "failed"
		task.mu.Unlock()
		return
	}
	env := os.Environ()
	for key, value := range envVars {
		env = append(env, key+"="+value)
	}
	if envVars["APP_PORT"] == "" {
		env = append(env, fmt.Sprintf("APP_PORT=%d", app.Port))
	}
	if envVars["APP_DOMAIN"] == "" {
		env = append(env, "APP_DOMAIN="+app.ID+".localhost")
	}
	if envVars["LOCAL_DOMAIN"] == "" {
		env = append(env, "LOCAL_DOMAIN=local")
	}
	if envVars["APP_DATA_DIR"] == "" {
		env = append(env, "APP_DATA_DIR="+appDir)
	}
	cmd := dockerComposeCommand(s.ctx, "-f", composeFile, "-p", app.ID, "up", "-d")
	cmd.Dir = appDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			logLine(line)
		}
	}
	task.mu.Lock()
	if err != nil {
		task.status = "failed"
	} else {
		task.status = "completed"
	}
	task.mu.Unlock()
}
func (s *LocalService) GetDockerAppInstall(ctx context.Context, id string) (AppInstallProgress, error) {
	if err := ctx.Err(); err != nil {
		return AppInstallProgress{}, err
	}
	return s.installProgress(id), nil
}
func (s *LocalService) installProgress(id string) AppInstallProgress {
	task := s.task(id)
	task.mu.RLock()
	defer task.mu.RUnlock()
	return AppInstallProgress{Status: task.status, Logs: append([]string(nil), task.logs...)}
}
func (s *LocalService) DockerAppAction(ctx context.Context, id, action string) error {
	if _, err := s.repo.GetAppTemplate(ctx, id); err != nil {
		return err
	}
	composeFile := filepath.Join(s.composeDir, id, "docker-compose.yml")
	if _, err := os.Stat(composeFile); err == nil {
		switch action {
		case "start":
			return runCompose(ctx, id, composeFile, "start")
		case "stop":
			return runCompose(ctx, id, composeFile, "stop")
		case "restart":
			return runCompose(ctx, id, composeFile, "restart")
		case "uninstall":
			err := runCompose(ctx, id, composeFile, "down", "-v")
			if err != nil {
				return err
			}
			return os.RemoveAll(filepath.Join(s.composeDir, id))
		}
		return fmt.Errorf("unsupported app action %q", action)
	}
	switch action {
	case "start":
		return s.sdk.StartDockerContainer(ctx, id)
	case "stop":
		return s.sdk.StopDockerContainer(ctx, id)
	case "restart":
		return s.sdk.RestartDockerContainer(ctx, id)
	case "uninstall":
		return s.sdk.RemoveDockerContainer(ctx, id)
	default:
		return fmt.Errorf("unsupported app action %q", action)
	}
}
func runCompose(ctx context.Context, name, file string, args ...string) error {
	all := []string{"-f", file, "-p", name}
	all = append(all, args...)
	cmd := dockerComposeCommand(ctx, all...)
	cmd.Dir = filepath.Dir(file)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), ctxErr)
		}
		return fmt.Errorf("docker compose %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return nil
}
func (s *LocalService) task(id string) *installTask {
	s.tasksMu.RLock()
	task := s.tasks[id]
	s.tasksMu.RUnlock()
	if task != nil {
		return task
	}
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()
	if task = s.tasks[id]; task == nil {
		task = &installTask{status: "idle", logs: []string{}}
		s.tasks[id] = task
	}
	return task
}

var _ Service = (*LocalService)(nil)
