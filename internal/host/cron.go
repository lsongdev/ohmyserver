package host

import (
	"context"

	cronweb "github.com/lsongdev/ohmyserver/modules/cron/web"
)

// CronModule returns the module-owned local runtime to the composition root.
func (s *Server) CronModule() *cronweb.LocalModule {
	if s.cronModule == nil {
		ctx := s.monitorContext
		if ctx == nil {
			ctx = context.Background()
		}
		s.cronModule = cronweb.NewLocalModule(ctx, s.db, nil)
	}
	return s.cronModule
}
