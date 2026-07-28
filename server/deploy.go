package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/lsongdev/ohmyserver/config"
)

type containerInspector interface {
	GetContainer(string) (types.ContainerJSON, error)
}

type deploymentRuntime interface {
	containerInspector
	BuildImageFromDirContext(context.Context, string, string, func(string)) error
	RemoveContainer(string) error
	CreateContainer(string, string, []string) (string, error)
	StartContainer(string) error
}

func (s *Server) projectContainerInspector() containerInspector {
	if s.deployRuntime != nil {
		return s.deployRuntime
	}
	return s.docker
}

// DeploymentExecutor runs one checkout, build, and deploy pipeline.
type DeploymentExecutor struct {
	server     *Server
	runtime    deploymentRuntime
	deployment *Deployment
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	workDir    string
}

func NewDeploymentExecutor(server *Server, deployment *Deployment) *DeploymentExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	runtime := deploymentRuntime(server.docker)
	if server.deployRuntime != nil {
		runtime = server.deployRuntime
	}
	return &DeploymentExecutor{
		server:     server,
		runtime:    runtime,
		deployment: deployment,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Execute runs the deployment and owns all terminal status transitions.
func (e *DeploymentExecutor) Execute() error {
	if err := e.server.UpdateDeploymentStatus(e.deployment.ID, DeploymentStatusRunning); err != nil {
		return fmt.Errorf("set deployment running: %w", err)
	}

	e.log("Deployment started")
	err := e.executePipeline()
	if err != nil {
		status := DeploymentStatusFailed
		message := fmt.Sprintf("Deployment failed: %v", err)
		if errors.Is(err, context.Canceled) || errors.Is(e.ctx.Err(), context.Canceled) {
			status = DeploymentStatusCancelled
			message = "Deployment cancelled"
		}
		if statusErr := e.server.UpdateDeploymentStatus(e.deployment.ID, status); statusErr != nil {
			return errors.Join(err, fmt.Errorf("set deployment %s: %w", status, statusErr))
		}
		e.log(message)
		return err
	}

	if err := e.server.UpdateDeploymentStatus(e.deployment.ID, DeploymentStatusCompleted); err != nil {
		return fmt.Errorf("set deployment completed: %w", err)
	}
	e.log("Deployment completed successfully")
	return nil
}

func (e *DeploymentExecutor) executePipeline() error {
	project, err := e.server.GetProjectByID(e.deployment.ProjectID)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if strings.TrimSpace(project.Repo) == "" {
		return errors.New("repository URL is required")
	}

	e.workDir = filepath.Join(
		config.WorkspaceDir,
		"projects",
		strconv.FormatInt(project.ID, 10),
		strconv.FormatInt(e.deployment.ID, 10),
	)
	if err := os.RemoveAll(e.workDir); err != nil {
		return fmt.Errorf("clean work directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(e.workDir), 0755); err != nil {
		return fmt.Errorf("create work directory parent: %w", err)
	}
	defer os.RemoveAll(e.workDir)

	e.log("Checking out source")
	requestedCommit := e.deployment.Commit
	if err := e.cloneRepository(project.Repo, requestedCommit); err != nil {
		return fmt.Errorf("checkout source: %w", err)
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}

	commit, commitMsg, author, err := e.readSourceMetadata()
	if err != nil {
		return fmt.Errorf("read source metadata: %w", err)
	}
	if requestedCommit != "" && !strings.EqualFold(commit, requestedCommit) {
		return fmt.Errorf("checked out commit %s, expected %s", commit, requestedCommit)
	}
	e.deployment.Commit = commit
	e.deployment.CommitMsg = commitMsg
	e.deployment.Author = author
	if err := e.server.UpdateDeploymentSource(e.deployment.ID, commit, commitMsg, author); err != nil {
		return fmt.Errorf("save source metadata: %w", err)
	}
	e.log(fmt.Sprintf("Checked out commit %s: %s", shortCommit(commit), commitMsg))

	if _, err := os.Stat(filepath.Join(e.workDir, "Dockerfile")); err != nil {
		if os.IsNotExist(err) {
			return errors.New("Dockerfile not found at repository root")
		}
		return fmt.Errorf("inspect Dockerfile: %w", err)
	}

	imageName := fmt.Sprintf("ohmyserver/project-%d:deployment-%d", project.ID, e.deployment.ID)
	e.log(fmt.Sprintf("Building image %s", imageName))
	if err := e.runtime.BuildImageFromDirContext(e.ctx, imageName, e.workDir, e.log); err != nil {
		return fmt.Errorf("build image: %w", err)
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	e.deployment.DockerImage = imageName

	containerName := fmt.Sprintf("ohmyserver-project-%d", project.ID)
	e.deployment.ContainerName = containerName
	if err := e.server.UpdateDeploymentArtifact(e.deployment.ID, imageName, containerName); err != nil {
		return fmt.Errorf("save build artifact: %w", err)
	}

	e.log(fmt.Sprintf("Deploying container %s", containerName))
	if err := e.deployContainer(imageName, containerName); err != nil {
		return fmt.Errorf("deploy container: %w", err)
	}
	return nil
}

func (e *DeploymentExecutor) cloneRepository(repositoryURL, commit string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git executable is required")
	}
	if commit == "" {
		return e.runGit("clone", "--depth=1", repositoryURL, e.workDir)
	}
	if err := e.runGit("init", e.workDir); err != nil {
		return err
	}
	if err := e.runGit("-C", e.workDir, "remote", "add", "origin", repositoryURL); err != nil {
		return err
	}
	if err := e.runGit("-C", e.workDir, "fetch", "--depth=1", "origin", commit); err != nil {
		return err
	}
	return e.runGit("-C", e.workDir, "checkout", "--detach", "FETCH_HEAD")
}

func (e *DeploymentExecutor) runGit(args ...string) error {
	cmd := exec.CommandContext(e.ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if e.ctx.Err() != nil {
		return e.ctx.Err()
	}
	return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
}

func (e *DeploymentExecutor) readSourceMetadata() (string, string, string, error) {
	values := make([]string, 0, 3)
	for _, format := range []string{"%H", "%s", "%an"} {
		cmd := exec.CommandContext(e.ctx, "git", "-C", e.workDir, "log", "-1", "--format="+format)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if e.ctx.Err() != nil {
				return "", "", "", e.ctx.Err()
			}
			return "", "", "", fmt.Errorf("git log: %w: %s", err, strings.TrimSpace(string(output)))
		}
		values = append(values, strings.TrimSpace(string(output)))
	}
	return values[0], values[1], values[2], nil
}

func (e *DeploymentExecutor) deployContainer(imageName, containerName string) error {
	if _, err := e.runtime.GetContainer(containerName); err == nil {
		e.log(fmt.Sprintf("Removing previous container %s", containerName))
		if err := e.runtime.RemoveContainer(containerName); err != nil {
			return fmt.Errorf("remove previous container: %w", err)
		}
	}

	var command []string
	if e.deployment.Command != "" {
		command = []string{"/bin/sh", "-c", e.deployment.Command}
	}
	containerID, err := e.runtime.CreateContainer(imageName, containerName, command)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if err := e.server.UpdateDeploymentContainerID(e.deployment.ID, containerID); err != nil {
		_ = e.runtime.RemoveContainer(containerID)
		return fmt.Errorf("save container ID: %w", err)
	}
	e.deployment.ContainerID = containerID

	if err := e.runtime.StartContainer(containerID); err != nil {
		_ = e.runtime.RemoveContainer(containerID)
		return fmt.Errorf("start container: %w", err)
	}
	e.log(fmt.Sprintf("Container started: %s", containerID))
	return nil
}

func (e *DeploymentExecutor) log(message string) {
	_ = e.server.AddDeploymentLog(
		e.deployment.ID,
		fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), message),
	)
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func (e *DeploymentExecutor) Cancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancel()
}
