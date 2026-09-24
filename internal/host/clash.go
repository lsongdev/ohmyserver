package host

import (
	"context"

	clashmodule "github.com/lsongdev/ohmyserver/modules/clash/web"
)

func (s *Server) initClashSchema() error {
	return clashmodule.NewRepository(s.db).Initialize(context.Background())
}

// ClashService constructs the module-owned implementation over the host's
// shared database connection pool.
func (s *Server) ClashService() clashmodule.Service {
	return clashmodule.NewManager(clashmodule.NewRepository(s.db))
}
