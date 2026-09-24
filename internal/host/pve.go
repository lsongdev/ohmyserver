package host

import (
	"context"
	pveweb "github.com/lsongdev/ohmyserver/modules/pve/web"
)

func (s *Server) PVERepository() *pveweb.Repository { return pveweb.NewRepository(s.db) }
func (s *Server) initPVESchema() error              { return s.PVERepository().Initialize(context.Background()) }
