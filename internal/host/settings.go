package host

import (
	"context"

	settingsmodule "github.com/lsongdev/ohmyserver/modules/settings/web"
)

func (s *Server) SettingsRepository() *settingsmodule.Repository {
	return settingsmodule.NewRepository(s.db)
}

func (s *Server) initSettingsSchema() error {
	return s.SettingsRepository().Initialize(context.Background())
}
