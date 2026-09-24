package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ComposeService struct{ root string }

func NewComposeService(root string) *ComposeService { return &ComposeService{root: root} }

func dockerComposeCommand(ctx context.Context, args ...string) *exec.Cmd {
	if executable, err := exec.LookPath("docker-compose"); err == nil {
		return exec.CommandContext(ctx, executable, args...)
	}
	if docker, err := exec.LookPath("docker"); err == nil {
		return exec.CommandContext(ctx, docker, append([]string{"compose"}, args...)...)
	}
	return exec.CommandContext(ctx, "docker-compose", args...)
}
func (s *ComposeService) ListDockerComposeProjects(ctx context.Context) ([]ComposeProject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := dockerComposeCommand(ctx, "ls", "--format", "json", "--all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("docker compose ls: %w", ctxErr)
		}
		return nil, fmt.Errorf("docker compose ls failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	var projects []ComposeProject
	if err := json.Unmarshal(output, &projects); err != nil {
		return nil, fmt.Errorf("decode docker compose project list: %w", err)
	}
	if projects == nil {
		projects = []ComposeProject{}
	}
	return projects, nil
}
func parseComposeContainers(output []byte) []map[string]any {
	containers := make([]map[string]any, 0)
	if json.Unmarshal(output, &containers) == nil {
		return containers
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		var item map[string]any
		if json.Unmarshal([]byte(line), &item) == nil {
			containers = append(containers, item)
		}
	}
	return containers
}
func (s *ComposeService) GetDockerComposeProject(ctx context.Context, name string) (*ComposeProjectDetail, error) {
	projects, err := s.ListDockerComposeProjects(ctx)
	if err != nil {
		return nil, err
	}
	var project *ComposeProject
	for i := range projects {
		if projects[i].Name == name {
			project = &projects[i]
			break
		}
	}
	if project == nil {
		return nil, errors.New("Compose project not found: " + name)
	}
	cmd := dockerComposeCommand(ctx, "-p", name, "ps", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("docker compose ps: %w", ctxErr)
		}
		return nil, fmt.Errorf("docker compose ps failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return &ComposeProjectDetail{Project: *project, Containers: parseComposeContainers(output)}, nil
}
func (s *ComposeService) DockerComposeAction(ctx context.Context, name, action string) error {
	project, err := s.GetDockerComposeProject(ctx, name)
	if err != nil {
		return err
	}
	switch action {
	case "up":
		return s.run(ctx, project.Project.Name, project.Project.ConfigFiles, "up", "-d")
	case "down":
		return s.run(ctx, project.Project.Name, project.Project.ConfigFiles, "down")
	case "restart":
		return s.run(ctx, project.Project.Name, project.Project.ConfigFiles, "restart")
	case "pull":
		return s.run(ctx, project.Project.Name, project.Project.ConfigFiles, "pull")
	default:
		return fmt.Errorf("unsupported compose action %q", action)
	}
}
func (s *ComposeService) run(ctx context.Context, name, configFile string, args ...string) error {
	commandArgs := []string{"-f", configFile}
	if name != "" {
		commandArgs = append(commandArgs, "-p", name)
	}
	commandArgs = append(commandArgs, args...)
	cmd := dockerComposeCommand(ctx, commandArgs...)
	cmd.Dir = filepath.Dir(configFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), ctxErr)
		}
		return fmt.Errorf("docker compose %s failed: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return nil
}
func (s *ComposeService) CreateDockerComposeProject(ctx context.Context, input ComposeProjectRequest) (*ComposeProject, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || filepath.Base(name) != name || strings.HasPrefix(name, ".") {
		return nil, fmt.Errorf("project name must be a simple directory name")
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') {
			return nil, fmt.Errorf("project name contains unsupported characters")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, name)
	switch input.Source {
	case "directory":
		dir = filepath.Clean(strings.TrimSpace(input.Directory))
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("directory must be an absolute path")
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("compose directory does not exist")
		}
	case "repository":
		if strings.TrimSpace(input.Repository) == "" {
			return nil, fmt.Errorf("repository URL is required")
		}
		if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
			return nil, err
		}
		args := []string{"clone", "--depth=1"}
		if branch := strings.TrimSpace(input.Branch); branch != "" {
			args = append(args, "--branch", branch)
		}
		args = append(args, strings.TrimSpace(input.Repository), dir)
		output, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git clone failed: %s: %w", strings.TrimSpace(string(output)), err)
		}
	case "file":
		if strings.TrimSpace(input.Compose) == "" {
			return nil, fmt.Errorf("compose configuration is required")
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(input.Compose), 0644); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("select file, directory, or repository source")
	}
	configFile := findComposeFile(dir)
	if configFile == "" {
		return nil, fmt.Errorf("no compose.yaml or docker-compose.yml found in project")
	}
	if err := s.run(ctx, name, configFile, "up", "-d"); err != nil {
		return nil, err
	}
	return &ComposeProject{Name: name, Status: "running", ConfigFiles: configFile}, nil
}
func findComposeFile(dir string) string {
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
