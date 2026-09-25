package host

import (
	"context"

	"github.com/lsongdev/ohmyserver/config"
	dockermodule "github.com/lsongdev/ohmyserver/modules/docker"
	dockerweb "github.com/lsongdev/ohmyserver/modules/docker/web"
)

func (s *Server) dockerModule() *dockermodule.LocalService {
	s.dockerServiceOnce.Do(func() {
		ctx := s.monitorContext
		if ctx == nil {
			ctx = context.Background()
		}
		s.dockerService = dockermodule.NewLocalService(ctx, s.docker, dockerweb.NewRepository(s.db), config.AppsDir, config.DockerComposeDir)
	})
	return s.dockerService
}

// DockerModuleService gives the composition root the module-owned service
// instead of exposing Server as the Docker handler's implementation.
func (s *Server) DockerModuleService() *dockermodule.LocalService { return s.dockerModule() }
