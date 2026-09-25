package host

import (
	"context"

	sshweb "github.com/lsongdev/ohmyserver/modules/ssh/web"
	terminalpkg "github.com/lsongdev/ohmyserver/pkg/terminal"
)

func (s *Server) localSSHModule() *sshweb.LocalModule {
	s.sshModuleOnce.Do(func() { s.sshModule = sshweb.NewLocalModule(s.db) })
	return s.sshModule
}

func (s *Server) SSHModule() *sshweb.LocalModule    { return s.localSSHModule() }
func (s *Server) SSHRepository() *sshweb.Repository { return s.localSSHModule().Repository() }
func (s *Server) SSHService() *sshweb.LocalService  { return s.localSSHModule().Service() }

// OpenSSHTerminal composes SSH transport with the shared terminal session contract.
func (s *Server) OpenSSHTerminal(ctx context.Context, id int64, cols, rows uint16) (terminalpkg.Session, error) {
	return s.SSHService().OpenTerminal(ctx, id, cols, rows)
}
