package host

import (
	"context"

	idracweb "github.com/lsongdev/ohmyserver/modules/idrac/web"
)

func (s *Server) localIDRACModule() *idracweb.LocalModule {
	s.idracModuleOnce.Do(func() { s.idracModule = idracweb.NewLocalModule(s.db) })
	return s.idracModule
}

func (s *Server) IDRACRepository() *idracweb.Repository { return s.localIDRACModule().Repository() }
func (s *Server) initIDRACSchema() error {
	return s.localIDRACModule().Initialize(context.Background())
}
