package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/config"
)

// DeploymentExecutor handles the deployment
type DeploymentExecutor struct {
	server     *Server
	deployment *Deployment
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	workDir    string
}

// NewDeploymentExecutor creates a new deployment executor
func NewDeploymentExecutor(server *Server, deployment *Deployment) *DeploymentExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	return &DeploymentExecutor{
		server:     server,
		deployment: deployment,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Execute runs the deployment
func (e *DeploymentExecutor) Execute() error {
	// Get project to access project name
	project, err := e.server.GetProjectByID(e.deployment.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// Update deployment status to running
	if err := e.server.UpdateDeploymentStatus(e.deployment.ID, DeploymentStatusRunning); err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	// Create working directory with project name under WorkspaceDir
	e.workDir = filepath.Join(config.WorkspaceDir, project.Name)
	if err := os.MkdirAll(e.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Deployment started", time.Now().Format("2006-01-02 15:04:05")))
	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Working directory: %s", time.Now().Format("2006-01-02 15:04:05"), e.workDir))

	// Clone repository if set
	if err := e.cloneRepository(); err != nil {
		e.server.UpdateDeploymentStatus(e.deployment.ID, DeploymentStatusFailed)
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Failed to clone repository: %v", time.Now().Format("2006-01-02 15:04:05"), err))
		return err
	}

	// Pull or build Docker image
	if err := e.prepareImage(); err != nil {
		e.server.UpdateDeploymentStatus(e.deployment.ID, DeploymentStatusFailed)
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Failed to prepare image: %v", time.Now().Format("2006-01-02 15:04:05"), err))
		return err
	}

	// Deploy container
	if err := e.deployContainer(); err != nil {
		e.server.UpdateDeploymentStatus(e.deployment.ID, DeploymentStatusFailed)
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Failed to deploy: %v", time.Now().Format("2006-01-02 15:04:05"), err))
		return err
	}

	// All completed successfully
	e.server.UpdateDeploymentStatus(e.deployment.ID, DeploymentStatusCompleted)
	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Deployment completed successfully", time.Now().Format("2006-01-02 15:04:05")))
	return nil
}

// cloneRepository clones the git repository
func (e *DeploymentExecutor) cloneRepository() error {
	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Preparing to checkout code...", time.Now().Format("2006-01-02 15:04:05")))

	// Get project to access repo URL
	project, err := e.server.GetProjectByID(e.deployment.ProjectID)
	if err != nil {
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Warning: Could not get project info: %v", time.Now().Format("2006-01-02 15:04:05"), err))
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Skipping repository checkout", time.Now().Format("2006-01-02 15:04:05")))
		return nil
	}

	// If no repo is set, skip checkout (for Docker-only deployments)
	if project.Repo == "" {
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] No repository set, skipping checkout", time.Now().Format("2006-01-02 15:04:05")))
		return nil
	}

	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Repository: %s", time.Now().Format("2006-01-02 15:04:05"), project.Repo))

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Git not available, skipping checkout", time.Now().Format("2006-01-02 15:04:05")))
		return nil
	}

	// Clean work directory
	os.RemoveAll(e.workDir)
	os.MkdirAll(e.workDir, 0755)

	// Clone repository
	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Cloning repository...", time.Now().Format("2006-01-02 15:04:05")))

	cmd := exec.CommandContext(e.ctx, "git", "clone", "--depth=1", project.Repo, e.workDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Clone error: %s", time.Now().Format("2006-01-02 15:04:05"), string(output)))
		return fmt.Errorf("failed to clone repository: %w, output: %s", err, string(output))
	}

	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Repository cloned successfully", time.Now().Format("2006-01-02 15:04:05")))
	return nil
}

// prepareImage pulls or builds the Docker image from the cloned repository
func (e *DeploymentExecutor) prepareImage() error {
	// Get project to access project name
	project, err := e.server.GetProjectByID(e.deployment.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	imageName := e.deployment.DockerImage

	// Build image from cloned repository
	if imageName == "" {
		// Use project name as image name
		imageName = project.Name + ":latest"
	}

	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Building image from repository: %s", time.Now().Format("2006-01-02 15:04:05"), imageName))

	// Build from the work directory (where repository was cloned)
	logCh, err := e.server.docker.BuildImageFromDir(imageName, e.workDir)
	if err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	// Read and log build output
	for log := range logCh {
		e.server.AddDeploymentLog(e.deployment.ID, log)
	}

	e.deployment.DockerImage = imageName
	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Image built successfully: %s", time.Now().Format("2006-01-02 15:04:05"), imageName))

	return nil
}

// deployContainer creates and starts the container
func (e *DeploymentExecutor) deployContainer() error {
	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Starting deployment...", time.Now().Format("2006-01-02 15:04:05")))

	containerName := e.deployment.ContainerName
	if containerName == "" {
		containerName = "deploy-" + strconv.FormatInt(e.deployment.ID, 10)
	}

	// Check if container already exists and remove it
	_, err := e.server.docker.GetContainer(containerName)
	if err == nil {
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Removing existing container: %s", time.Now().Format("2006-01-02 15:04:05"), containerName))
		if err := e.server.docker.RemoveContainer(containerName); err != nil {
			return fmt.Errorf("failed to remove existing container: %w", err)
		}
	}

	// Use image default command (empty command uses CMD from Dockerfile)
	var cmd []string
	if e.deployment.Command != "" {
		cmd = []string{"/bin/sh", "-c", e.deployment.Command}
	}

	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Creating container: %s", time.Now().Format("2006-01-02 15:04:05"), containerName))

	// Create container without volume mount (files are already in the image via Dockerfile COPY)
	// Pass empty strings for hostPath, containerPath, and port to use image defaults
	containerID, err := e.server.docker.CreateContainer(
		e.deployment.DockerImage,
		containerName,
		cmd,
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Container created: %s", time.Now().Format("2006-01-02 15:04:05"), containerID))

	// Save container ID to deployment
	if err := e.server.UpdateDeploymentContainerID(e.deployment.ID, containerID); err != nil {
		e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Warning: Failed to save container ID: %v", time.Now().Format("2006-01-02 15:04:05"), err))
	}
	e.deployment.ContainerID = containerID

	if err := e.server.docker.StartContainer(containerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	e.server.AddDeploymentLog(e.deployment.ID, fmt.Sprintf("[%s] Container started successfully", time.Now().Format("2006-01-02 15:04:05")))
	return nil
}

// Cancel stops the deployment execution
func (e *DeploymentExecutor) Cancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancel()
}
