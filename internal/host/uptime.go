package host

import (
	"context"

	uptimeweb "github.com/lsongdev/ohmyserver/modules/uptime/web"
)

// UptimeRepository supplies the shell's shared SQLite connection to the module.
func (s *Server) UptimeRepository() *uptimeweb.Repository {
	if s.uptimeRepository == nil {
		s.uptimeRepository = uptimeweb.NewRepository(s.db)
	}
	return s.uptimeRepository
}

func (s *Server) initUptimeSchema() error {
	return s.UptimeRepository().Initialize(context.Background())
}
