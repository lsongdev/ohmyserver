package host

import (
	"context"
	"fmt"
	"time"

	dockerweb "github.com/lsongdev/ohmyserver/modules/docker/web"
)

const appsSyncTimeout = 2 * time.Minute

func (s *Server) initAppsSchema() error {
	return dockerweb.NewRepository(s.db).InitializeAppCatalog(context.Background())
}

func (s *Server) syncDockerAppCatalog() error {
	ctx, cancel := context.WithTimeout(context.Background(), appsSyncTimeout)
	defer cancel()
	if err := s.dockerModule().SyncDockerApps(ctx); err != nil {
		if loadErr := s.dockerModule().LoadDockerApps(ctx); loadErr != nil {
			return fmt.Errorf("sync app catalog: %v; load cached catalog: %w", err, loadErr)
		}
		return fmt.Errorf("sync app catalog (using cached catalog): %w", err)
	}
	return nil
}
