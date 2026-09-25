package host

import (
	"context"

	openwrtweb "github.com/lsongdev/ohmyserver/modules/openwrt/web"
)

func (s *Server) OpenWrtRepository() *openwrtweb.Repository {
	return openwrtweb.NewRepository(s.db)
}

func (s *Server) initOpenWrtSchema() error {
	return s.OpenWrtRepository().Initialize(context.Background())
}
