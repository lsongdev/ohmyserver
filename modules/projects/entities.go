package projects

import (
	"errors"
	"time"
)

type Input struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Repo          string `json:"repo"`
	WebhookBranch string `json:"webhook_branch"`
	WebhookSecret string `json:"webhook_secret"`
}

type RuntimeStatus struct {
	Status        string `json:"status"`
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	IsRunning     bool   `json:"is_running"`
	Error         string `json:"error,omitempty"`
}

var ErrDeploymentInProgress = errors.New("deployment already in progress")

type Project struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Repo          string    `json:"repo"`
	WebhookSecret string    `json:"-"`
	WebhookBranch string    `json:"webhook_branch"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DeploymentStatus string

const (
	DeploymentPending   DeploymentStatus = "pending"
	DeploymentRunning   DeploymentStatus = "running"
	DeploymentCompleted DeploymentStatus = "completed"
	DeploymentFailed    DeploymentStatus = "failed"
	DeploymentCancelled DeploymentStatus = "cancelled"
)

type Deployment struct {
	ID            int64            `json:"id"`
	ProjectID     int64            `json:"project_id"`
	Status        DeploymentStatus `json:"status"`
	DockerImage   string           `json:"docker_image"`
	Dockerfile    string           `json:"dockerfile"`
	ContainerName string           `json:"container_name"`
	ContainerID   string           `json:"container_id"`
	Command       string           `json:"command"`
	Port          int              `json:"port"`
	Commit        string           `json:"commit"`
	CommitMsg     string           `json:"commit_msg"`
	Author        string           `json:"author"`
	TriggeredBy   string           `json:"triggered_by"`
	StartedAt     *time.Time       `json:"started_at"`
	CompletedAt   *time.Time       `json:"completed_at"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

type DeploymentLog struct {
	ID           int64     `json:"id"`
	DeploymentID int64     `json:"deployment_id"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}
