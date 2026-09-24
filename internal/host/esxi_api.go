package host

import (
	"context"

	esxiweb "github.com/lsongdev/ohmyserver/modules/esxi/web"
)

func (s *Server) localESXiModule() *esxiweb.LocalModule {
	s.esxiModuleOnce.Do(func() { s.esxiModule = esxiweb.NewLocalModule(s.db) })
	return s.esxiModule
}

func (s *Server) ESXiRepository() *esxiweb.Repository { return s.localESXiModule().Repository() }
func (s *Server) initESXiServerSchema() error {
	return s.localESXiModule().Initialize(context.Background())
}
