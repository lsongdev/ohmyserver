package host

import (
	"context"

	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
	forwardweb "github.com/lsongdev/ohmyserver/modules/forward/web"
	projectsweb "github.com/lsongdev/ohmyserver/modules/projects/web"
	sqlite "modernc.org/sqlite"
)

func init() {
	// SQLite allows concurrent readers but serializes writers. Configure every
	// pooled connection to wait briefly for an active writer instead of failing
	// immediately with SQLITE_BUSY during monitor/API overlap.
	sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, _ string) error {
		_, err := conn.ExecContext(context.Background(), `PRAGMA busy_timeout=5000`, nil)
		return err
	})
}

// initSchema creates the necessary tables via registered hooks
func (s *Server) initSchema() error {
	if err := s.initializeModuleSchemas(); err != nil {
		return err
	}
	if err := s.runLegacyMigrations(); err != nil {
		return err
	}
	return nil
}

func (s *Server) projectRepository() *projectsweb.Repository {
	return projectsweb.NewRepository(s.db)
}

func (s *Server) ProjectsRepository() *projectsweb.Repository { return s.projectRepository() }

func (s *Server) DNSRepository() *dnsweb.Repository { return dnsweb.NewRepository(s.db) }

// importLegacyDNSProviders transfers legacy provider rows into the DNS-owned
// repository. The DNS module itself never queries the old table.
func (s *Server) importLegacyDNSProviders() error {
	ctx := context.Background()
	repository := s.DNSRepository()
	var legacyExists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='dns_providers')`).Scan(&legacyExists); err != nil {
		return err
	}
	if !legacyExists {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,type,config FROM dns_providers`)
	if err != nil {
		return err
	}
	providers := make([]dnsmodule.Provider, 0)
	for rows.Next() {
		var provider dnsmodule.Provider
		if err := rows.Scan(&provider.ID, &provider.Name, &provider.Type, &provider.Config); err != nil {
			rows.Close()
			return err
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return repository.ImportProviders(ctx, providers)
}

func (s *Server) ForwardRepository() *forwardweb.Repository {
	return forwardweb.NewRepository(s.db)
}

// initTCPListenerSchema creates the TCP listener tables
func (s *Server) initTCPListenerSchema() error {
	return s.ForwardRepository().Initialize(context.Background())
}

// SSH server persistence is kept separate from the cron scheduler schema.
func (s *Server) initSSHServerSchema() error {
	return s.localSSHModule().Initialize(context.Background())
}

func (s *Server) initACMESchema() error {
	repository := s.ACMERepository()
	if err := repository.InitializeModule(context.Background()); err != nil {
		return err
	}
	return repository.RecoverInterruptedIssuances(context.Background())
}
