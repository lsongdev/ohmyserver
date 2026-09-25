package host

import (
	"context"

	"github.com/lsongdev/ohmyserver/config"
	dockermodule "github.com/lsongdev/ohmyserver/modules/docker"
	projectsweb "github.com/lsongdev/ohmyserver/modules/projects/web"
)

type deploymentRuntimeAdapter struct{ server *Server }

func (s *Server) ProjectDeploymentManager() *projectsweb.DeploymentManager {
	s.projectDeploymentManagerOnce.Do(func() {
		ctx := s.monitorContext
		if ctx == nil {
			ctx = context.Background()
		}
		s.projectDeployments = projectsweb.NewDeploymentManager(ctx, s.projectRepository(), deploymentRuntimeAdapter{server: s}, config.WorkspaceDir)
	})
	return s.projectDeployments
}

func (a deploymentRuntimeAdapter) runtime() projectsweb.DeploymentRuntime {
	if a.server.deployRuntime != nil {
		return a.server.deployRuntime
	}
	return dockerDeploymentRuntime{service: a.server.dockerModule()}
}
func (a deploymentRuntimeAdapter) InspectContainer(ctx context.Context, name string) (projectsweb.ContainerState, error) {
	return a.runtime().InspectContainer(ctx, name)
}
func (a deploymentRuntimeAdapter) BuildImageFromDirContext(ctx context.Context, name, dir string, onLog func(string)) error {
	return a.runtime().BuildImageFromDirContext(ctx, name, dir, onLog)
}
func (a deploymentRuntimeAdapter) RemoveContainer(ctx context.Context, name string) error {
	return a.runtime().RemoveContainer(ctx, name)
}
func (a deploymentRuntimeAdapter) CreateContainer(ctx context.Context, imageName, name string, command []string) (string, error) {
	return a.runtime().CreateContainer(ctx, imageName, name, command)
}
func (a deploymentRuntimeAdapter) StartContainer(ctx context.Context, name string) error {
	return a.runtime().StartContainer(ctx, name)
}

type dockerDeploymentRuntime struct{ service *dockermodule.LocalService }

func (d dockerDeploymentRuntime) InspectContainer(ctx context.Context, name string) (projectsweb.ContainerState, error) {
	container, err := d.service.InspectDockerContainer(ctx, name)
	if err != nil {
		return projectsweb.ContainerState{}, err
	}
	return projectsweb.ContainerState{ID: container.ID, Running: container.State.Running, Dead: container.State.Dead, OOMKilled: container.State.OOMKilled, ExitCode: container.State.ExitCode}, nil
}
func (d dockerDeploymentRuntime) BuildImageFromDirContext(ctx context.Context, name, dir string, onLog func(string)) error {
	return d.service.BuildDockerImage(ctx, dockermodule.BuildImageRequest{Name: name, SourceType: "directory", BuildDir: dir}, func(line string) error {
		onLog(line)
		return nil
	})
}
func (d dockerDeploymentRuntime) RemoveContainer(ctx context.Context, name string) error {
	return d.service.RemoveDockerContainer(ctx, name)
}
func (d dockerDeploymentRuntime) CreateContainer(ctx context.Context, image, name string, command []string) (string, error) {
	return d.service.CreateContainer(ctx, image, name, command)
}
func (d dockerDeploymentRuntime) StartContainer(ctx context.Context, name string) error {
	return d.service.StartDockerContainer(ctx, name)
}
