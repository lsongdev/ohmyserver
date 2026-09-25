package host

import (
	"context"
	"fmt"
	"strings"

	dhcpmodule "github.com/lsongdev/ohmyserver/modules/dhcp"
	dhcpweb "github.com/lsongdev/ohmyserver/modules/dhcp/web"
)

func (s *Server) initDHCPSchema() error {
	ctx := context.Background()
	repository := s.dhcpRepository()
	if err := repository.Initialize(ctx); err != nil {
		return err
	}
	var legacyExists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='dhcp_reservations')`).Scan(&legacyExists); err != nil {
		return err
	}
	if !legacyExists {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,mac,ip,hostname,description FROM dhcp_reservations`)
	if err != nil {
		return err
	}
	leases := make([]dhcpmodule.StaticLease, 0)
	for rows.Next() {
		var lease dhcpmodule.StaticLease
		if err := rows.Scan(&lease.ID, &lease.MAC, &lease.IP, &lease.Hostname, &lease.Description); err != nil {
			rows.Close()
			return err
		}
		leases = append(leases, lease)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := repository.ImportStaticLeases(ctx, leases); err != nil {
		return fmt.Errorf("migrate DHCP static leases: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `DROP TABLE dhcp_reservations`)
	return err
}
func (s *Server) localDHCPModule() *dhcpweb.LocalModule {
	s.dhcpModuleOnce.Do(func() {
		s.dhcpModule = dhcpweb.NewLocalModule(
			s.dhcpLifecycleContext(), s.db, s.applyDHCPPolicy, dhcpServiceAdapter{server: s},
		)
	})
	return s.dhcpModule
}

func (s *Server) dhcpRepository() *dhcpweb.Repository { return s.localDHCPModule().Repository() }
func (s *Server) DHCPRepository() *dhcpweb.Repository { return s.dhcpRepository() }
func (s *Server) dhcpModuleService() *dhcpweb.LocalService {
	return s.localDHCPModule().Service()
}
func (s *Server) applyDHCPPolicy(ctx context.Context, item dhcpmodule.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	previous, err := s.getServicePreferenceContext(ctx, "dhcp", item.Enabled)
	if err != nil {
		return err
	}
	rollback := func() {
		_ = s.setServiceDesiredContext(context.WithoutCancel(ctx), "dhcp", previous.Enabled)
	}
	if err := s.setServiceDesiredContext(ctx, "dhcp", item.Enabled); err != nil {
		return err
	}
	if item.Enabled {
		conflicts, conflictErr := s.configuredServiceConflicts("dhcp")
		if conflictErr != nil {
			rollback()
			return conflictErr
		}
		if len(conflicts) > 0 {
			rollback()
			return fmt.Errorf("listener conflict: %s is also used by %s", conflicts[0].Endpoint, strings.Join(conflicts[0].Services, ", "))
		}
	}
	if err := ctx.Err(); err != nil {
		rollback()
		return err
	}
	return nil
}
type dhcpServiceAdapter struct{ server *Server }

func (a dhcpServiceAdapter) UpdateDHCPConfiguration(ctx context.Context, item dhcpweb.Settings) error {
  err := a.server.dhcpModuleService().UpdateDHCPConfiguration(ctx, item)
  a.server.setServiceError("dhcp", err)
  return err
}
func (a dhcpServiceAdapter) SaveReservation(ctx context.Context, id int64, item dhcpweb.StaticLease) error {
  return a.server.dhcpModuleService().SaveReservation(ctx, id, item)
}
func (a dhcpServiceAdapter) DeleteReservation(ctx context.Context, id int64) error {
  return a.server.dhcpModuleService().DeleteReservation(ctx, id)
}
func (a dhcpServiceAdapter) DeleteDHCPLease(ctx context.Context, id int64) error {
  return a.server.dhcpModuleService().DeleteDHCPLease(ctx, id)
}
func (a dhcpServiceAdapter) DHCPRuntimeStatus(ctx context.Context) (any, error) {
  return a.server.dhcpModuleService().DHCPRuntimeStatus(ctx)
}
func (a dhcpServiceAdapter) DHCPAction(ctx context.Context, action string) error {
  err := a.server.dhcpModuleService().DHCPAction(ctx, action)
  a.server.setServiceError("dhcp", err)
  return err
}

func (s *Server) dhcpLifecycleContext() context.Context {
  if s.monitorContext != nil {
    return s.monitorContext
  }
  return context.Background()
}
