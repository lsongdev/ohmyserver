package host

import (
	"context"
	speedtestweb "github.com/lsongdev/ohmyserver/modules/speedtest/web"
)

func (s *Server) SpeedtestRepository() *speedtestweb.Repository {
	return speedtestweb.NewRepository(s.db)
}
func (s *Server) initSpeedtestSchema() error {
	return s.SpeedtestRepository().Initialize(context.Background())
}
