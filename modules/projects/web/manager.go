package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
)

type DeploymentManager struct {
	ctx       context.Context
	repo      *Repository
	runtime   DeploymentRuntime
	workspace string
	mu        sync.Mutex
	active    map[int64]*DeploymentExecutor
	wg        sync.WaitGroup
}

func NewDeploymentManager(ctx context.Context, repo *Repository, runtime DeploymentRuntime, workspace string) *DeploymentManager {
	return &DeploymentManager{ctx: ctx, repo: repo, runtime: runtime, workspace: workspace, active: make(map[int64]*DeploymentExecutor)}
}

func (m *DeploymentManager) StartProjectDeployment(ctx context.Context, project *Project, triggeredBy string) (*Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	active, err := m.activeDeployment(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("check active deployment: %w", err)
	}
	if active != nil {
		return nil, fmt.Errorf("%w: deployment #%d is %s", ErrDeploymentInProgress, active.ID, active.Status)
	}
	deployment, err := m.repo.CreateDeployment(ctx, Deployment{ProjectID: project.ID, TriggeredBy: triggeredBy})
	if err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}
	m.launchLocked(deployment)
	return deployment, nil
}

func (m *DeploymentManager) StartDeploymentAPI(ctx context.Context, projectID int64) (Deployment, error) {
	project, err := m.repo.Get(ctx, projectID)
	if err != nil {
		return Deployment{}, err
	}
	if project.Repo == "" {
		return Deployment{}, errors.New("repository URL is required")
	}
	deployment, err := m.StartProjectDeployment(ctx, project, "Manual")
	if err != nil {
		return Deployment{}, err
	}
	return *deployment, nil
}

func (m *DeploymentManager) ProjectRuntime(ctx context.Context, projectID int64) (RuntimeStatus, error) {
	if _, err := m.repo.Get(ctx, projectID); err != nil {
		return RuntimeStatus{}, err
	}
	status := RuntimeStatus{Status: "not_deployed"}
	deployment, err := m.repo.LatestDeployment(ctx, projectID, true)
	if errors.Is(err, sql.ErrNoRows) {
		deployment, err = m.repo.LatestDeployment(ctx, projectID, false)
		if errors.Is(err, sql.ErrNoRows) {
			return status, nil
		}
		if err != nil {
			return status, err
		}
		switch deployment.Status {
		case DeploymentPending, DeploymentRunning:
			status.Status = "starting"
		case DeploymentFailed:
			status.Status, status.Error = "error", "Deployment failed"
		}
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.ContainerID, status.ContainerName = deployment.ContainerID, deployment.ContainerName
	lookup := deployment.ContainerID
	if lookup == "" {
		lookup = deployment.ContainerName
	}
	if lookup == "" {
		if deployment.Status == DeploymentCompleted {
			status.Status = "stopped"
		}
		return status, nil
	}
	container, err := m.runtime.InspectContainer(ctx, lookup)
	if err != nil {
		if deployment.Status == DeploymentCompleted {
			status.Status = "stopped"
		} else {
			status.Status, status.Error = "error", "Container not found"
		}
		return status, nil
	}
	status.ContainerID = container.ID
	status.IsRunning = container.Running
	switch {
	case container.Running:
		status.Status = "running"
	case container.Dead || container.OOMKilled:
		status.Status, status.Error = "error", "Container died unexpectedly"
	case container.ExitCode != 0:
		status.Status, status.Error = "error", fmt.Sprintf("Container exited with code %d", container.ExitCode)
	default:
		status.Status = "stopped"
	}
	return status, nil
}

func (m *DeploymentManager) StartGitHubDeployment(ctx context.Context, project *Project, commit, commitMsg, author, deliveryID string) (*Deployment, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, err := m.repo.DeploymentByDelivery(ctx, deliveryID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("check webhook delivery: %w", err)
	}
	if err == nil {
		return existing, true, nil
	}
	active, err := m.activeDeployment(ctx, project.ID)
	if err != nil {
		return nil, false, fmt.Errorf("check active deployment: %w", err)
	}
	if active != nil {
		return nil, false, fmt.Errorf("%w: deployment #%d is %s", ErrDeploymentInProgress, active.ID, active.Status)
	}
	deployment, err := m.repo.CreateGitHubDeployment(ctx, Deployment{ProjectID: project.ID, Commit: commit, CommitMsg: commitMsg, Author: author}, deliveryID)
	if err != nil {
		return nil, false, fmt.Errorf("create GitHub deployment: %w", err)
	}
	m.launchLocked(deployment)
	return deployment, false, nil
}

func (m *DeploymentManager) Cancel(ctx context.Context, projectID, deploymentID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deployment, err := m.repo.GetDeployment(ctx, deploymentID)
	if err != nil || deployment.ProjectID != projectID {
		return errors.New("deployment not found")
	}
	m.mu.Lock()
	executor := m.active[deploymentID]
	m.mu.Unlock()
	if executor != nil {
		executor.Cancel()
		return nil
	}
	if deployment.Status == DeploymentCompleted || deployment.Status == DeploymentFailed || deployment.Status == DeploymentCancelled {
		return errors.New("deployment already finished")
	}
	if err := m.repo.UpdateDeploymentStatus(ctx, deploymentID, DeploymentCancelled); err != nil {
		return err
	}
	return m.repo.AddDeploymentLog(ctx, deploymentID, "Deployment cancelled without an active executor")
}

func (m *DeploymentManager) CancelDeploymentAPI(ctx context.Context, projectID, deploymentID int64) error {
	return m.Cancel(ctx, projectID, deploymentID)
}

func (m *DeploymentManager) activeDeployment(ctx context.Context, projectID int64) (*Deployment, error) {
	deployment, err := m.repo.ActiveDeployment(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return deployment, err
}

func (m *DeploymentManager) launchLocked(deployment *Deployment) {
	executor := NewDeploymentExecutor(m.ctx, m.repo, m.runtime, m.workspace, deployment)
	m.active[deployment.ID] = executor
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.mu.Lock()
			delete(m.active, deployment.ID)
			m.mu.Unlock()
		}()
		if err := executor.Execute(); err != nil {
			log.Printf("project deployment %d failed: %v", deployment.ID, err)
		}
	}()
}

func (m *DeploymentManager) Close() {
	m.mu.Lock()
	for _, executor := range m.active {
		executor.Cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
