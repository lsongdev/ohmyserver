package web

import projectsmodule "github.com/lsongdev/ohmyserver/modules/projects"

type Input = projectsmodule.Input
type RuntimeStatus = projectsmodule.RuntimeStatus
type Project = projectsmodule.Project
type Deployment = projectsmodule.Deployment
type DeploymentLog = projectsmodule.DeploymentLog
type DeploymentStatus = projectsmodule.DeploymentStatus

const (
	DeploymentPending   = projectsmodule.DeploymentPending
	DeploymentRunning   = projectsmodule.DeploymentRunning
	DeploymentCompleted = projectsmodule.DeploymentCompleted
	DeploymentFailed    = projectsmodule.DeploymentFailed
	DeploymentCancelled = projectsmodule.DeploymentCancelled
)

var ErrDeploymentInProgress = projectsmodule.ErrDeploymentInProgress
