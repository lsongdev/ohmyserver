package host

import (
	"context"
	ipmiweb "github.com/lsongdev/ohmyserver/modules/ipmi/web"
)

func (s *Server) localIPMIModule() *ipmiweb.LocalModule {
	s.ipmiModuleOnce.Do(func() { s.ipmiModule = ipmiweb.NewLocalModule(s.db) })
	return s.ipmiModule
}

func (s *Server) IPMIRepository() *ipmiweb.Repository { return s.localIPMIModule().Repository() }
func (s *Server) initIPMISchema() error               { return s.localIPMIModule().Initialize(context.Background()) }
