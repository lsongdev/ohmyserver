package host

import (
	"context"

	"github.com/lsongdev/ohmyserver/buildinfo"
	systemmodule "github.com/lsongdev/ohmyserver/modules/system"
	systemweb "github.com/lsongdev/ohmyserver/modules/system/web"
)

func (s *Server) SystemModuleService() systemweb.Service {
	listen, database, dockerMirror := "", "", ""
	if s.config != nil {
		listen = s.config.Listen
		database = s.config.Database.Driver + " - " + s.config.Database.Path
		dockerMirror = s.config.Docker.RegistryMirror
	}
	return systemmodule.NewService(systemmodule.ServiceOptions{
		Info: systemmodule.Info{
			ProgramName: "OhMyServer", ProgramVersion: buildinfo.Version,
			ProgramCommit: buildinfo.ShortCommit(), ProgramDesc: "A modern server management platform",
			BuildTime: buildinfo.BuildTime, Listen: listen,
			Database:     database,
			DockerMirror: dockerMirror,
			Credits: []systemmodule.Credit{
				{Name: "Go", URL: "https://golang.org", License: "BSD"},
				{Name: "SQLite", URL: "https://sqlite.org", License: "Public Domain"},
				{Name: "Docker", URL: "https://docker.com", License: "Apache 2.0"},
				{Name: "Font Awesome", URL: "https://fontawesome.com", License: "CC BY 4.0"},
			},
		},
		StartedAt: s.startedAt,
		Queue: func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if s.scheduleUpdate == nil {
				return systemmodule.ErrUpdateUnavailable
			}
			return s.scheduleUpdate()
		},
	})
}
