package host

import (
	"context"

	yeelightweb "github.com/lsongdev/ohmyserver/modules/yeelight/web"
)

func (s *Server) YeelightRepository() *yeelightweb.Repository { return yeelightweb.NewRepository(s.db) }
func (s *Server) initYeelightSchema() error {
	return s.YeelightRepository().Initialize(context.Background())
}
