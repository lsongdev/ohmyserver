package host

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	terminalmodule "github.com/lsongdev/ohmyserver/modules/terminal/web"
	terminalpkg "github.com/lsongdev/ohmyserver/pkg/terminal"
)

// OpenTerminal composes the application's available terminal backends without
// exposing their persistence details to the reusable terminal module.
func (s *Server) OpenTerminal(ctx context.Context, kind, id string, cols, rows uint16) (terminalpkg.Session, error) {
	switch kind {
	case "local":
		return (terminalpkg.Local{}).Open(ctx, cols, rows)
	case "ssh":
		serverID, err := strconv.ParseInt(id, 10, 64)
		if err != nil || serverID < 1 {
			return nil, fmt.Errorf("invalid SSH server id")
		}
		return s.OpenSSHTerminal(ctx, serverID, cols, rows)
	case "docker":
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("container id is required")
		}
		return s.docker.OpenTerminal(ctx, id, cols, rows)
	default:
		return nil, fmt.Errorf("unsupported terminal backend %q", kind)
	}
}

type terminalServiceAdapter struct{ server *Server }

func (a terminalServiceAdapter) OpenTerminal(ctx context.Context, kind, id string, cols, rows uint16) (terminalpkg.Session, error) {
	return a.server.OpenTerminal(ctx, kind, id, cols, rows)
}

func (s *Server) TerminalModuleService() terminalmodule.Service {
	return terminalServiceAdapter{server: s}
}

var _ terminalmodule.Service = terminalServiceAdapter{}
