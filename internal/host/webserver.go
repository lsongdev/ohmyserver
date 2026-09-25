package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"

	webmodule "github.com/lsongdev/ohmyserver/modules/webserver"
	webruntime "github.com/lsongdev/ohmyserver/modules/webserver/web"
	webrepo "github.com/lsongdev/ohmyserver/modules/webserver/web/repository"
)

func (s *Server) initWebServerSchema() error {
	ctx := context.Background()
	if err := s.WebServerRepository().Initialize(ctx); err != nil {
		return err
	}
	// The application adapter detaches legacy Web Server references to ACME
	// certificates. Feature repositories intentionally never inspect another
	// module's tables or foreign-key names.
	if err := s.migrateLegacyWebCertificateReferences(ctx); err != nil {
		return err
	}
	return s.WebServerRepository().EnsureDefaultListener(ctx, ":8081")
}

func (s *Server) migrateLegacyWebCertificateReferences(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	hasLegacyReference := func(table string) (bool, error) {
		if table != "web_listeners" && table != "web_hosts" {
			return false, fmt.Errorf("invalid Web Server table %q", table)
		}
		rows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_list("+table+")")
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, seq int
			var target, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &seq, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				return false, err
			}
			if target == "certificates" {
				return true, nil
			}
		}
		return false, rows.Err()
	}
	listenerFK, err := hasLegacyReference("web_listeners")
	if err != nil {
		return err
	}
	hostFK, err := hasLegacyReference("web_hosts")
	if err != nil {
		return err
	}
	if !listenerFK && !hostFK {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if listenerFK {
		statements := []string{
			`CREATE TABLE web_listeners_new (id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,address TEXT NOT NULL,protocol TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,min_tls_version TEXT NOT NULL DEFAULT 'TLS1.2',default_certificate_id INTEGER,trusted_proxies TEXT NOT NULL DEFAULT '',created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
			`INSERT INTO web_listeners_new(id,name,address,protocol,enabled,min_tls_version,default_certificate_id,trusted_proxies,created_at,updated_at) SELECT id,name,address,protocol,enabled,min_tls_version,default_certificate_id,trusted_proxies,created_at,updated_at FROM web_listeners`,
			`DROP TABLE web_listeners`,
			`ALTER TABLE web_listeners_new RENAME TO web_listeners`,
			`CREATE INDEX IF NOT EXISTS idx_web_listeners_enabled ON web_listeners(enabled)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	if hostFK {
		statements := []string{
			`CREATE TABLE web_hosts_new(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,hostnames TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,certificate_id INTEGER,force_https INTEGER NOT NULL DEFAULT 0,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
			`INSERT INTO web_hosts_new(id,name,hostnames,enabled,certificate_id,force_https,created_at,updated_at) SELECT id,name,hostnames,enabled,certificate_id,force_https,created_at,updated_at FROM web_hosts`,
			`DROP TABLE web_hosts`,
			`ALTER TABLE web_hosts_new RENAME TO web_hosts`,
			`CREATE INDEX IF NOT EXISTS idx_web_hosts_enabled ON web_hosts(enabled)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// migrateLegacyWebWorkerScripts is an application-level bridge for databases
// created before worker scripts became a field owned by web_routes. The Web
// Server repository never reads the retired workers table.
func (s *Server) migrateLegacyWebWorkerScripts() error {
	ctx := context.Background()
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='workers'`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	hasWorkerColumn := func(column string) (bool, error) {
		rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(workers)`)
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, kind string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
				return false, err
			}
			if name == column {
				return true, nil
			}
		}
		return false, rows.Err()
	}
	for _, column := range []string{"id", "script"} {
		found, err := hasWorkerColumn(column)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
	}
	repo := s.WebServerRepository()
	bindings, err := repo.LegacyWorkerBindings(ctx)
	if err != nil {
		return err
	}
	for _, item := range bindings {
		var script string
		if err := s.db.QueryRowContext(ctx, `SELECT script FROM workers WHERE id=?`, item.WorkerID).Scan(&script); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		if err := repo.SetLegacyWorkerScript(ctx, item.RouteID, script); err != nil {
			return err
		}
	}
	return nil
}

type webserverCertificateAdapter struct{ source webCertificateSource }

func (a webserverCertificateAdapter) ListSummaries(ctx context.Context) ([]webmodule.CertificateSummary, error) {
	items, err := a.source.ListSummaries(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]webmodule.CertificateSummary, 0, len(items))
	for _, item := range items {
		result = append(result, webmodule.CertificateSummary{ID: item.ID, Domains: item.Domains, Status: item.Status})
	}
	return result, nil
}

func (a webserverCertificateAdapter) LoadTLSMaterial(ctx context.Context, id int64) (webmodule.TLSMaterial, error) {
	item, err := a.source.LoadTLSMaterial(ctx, id)
	return webmodule.TLSMaterial{CertificatePEM: item.CertPEM, PrivateKeyPEM: item.PrivateKeyPEM, Status: item.Status}, err
}

func (s *Server) WebServerRepository() *webrepo.Repository { return webrepo.NewRepository(s.db) }

func (s *Server) WebServerModuleService() *webruntime.LocalService {
	return webruntime.NewLocalService(s.WebServerRepository(), s.webserverRuntime(), webserverCertificateAdapter{source: s.webCertificateSource()}, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.serviceDesiredEnabled("web", true) {
			return s.validateServiceStart("web")
		}
		return nil
	})
}

func (s *Server) webserverRuntime() *webruntime.Runtime {
	s.webRuntimeMu.Lock()
	defer s.webRuntimeMu.Unlock()
	if s.webRuntime == nil {
		ctx := s.monitorContext
		if ctx == nil {
			ctx = context.Background()
		}
		s.webRuntime = webruntime.NewRuntime(ctx, s.WebServerRepository(), webserverCertificateAdapter{source: s.webCertificateSource()}, s)
	}
	return s.webRuntime
}

func (s *Server) applyYabber(ctx context.Context) error {
	if s.serviceDesiredEnabled("web", true) {
		if err := s.validateServiceStart("web"); err != nil {
			return err
		}
	}
	return s.webserverRuntime().Apply(ctx)
}

func (s *Server) stopWebServer() error {
	return s.webserverRuntime().Close()
}

func (s *Server) WebServerMetricsHandler(w http.ResponseWriter, r *http.Request) {
	s.webserverRuntime().MetricsHandler().ServeHTTP(w, r)
}

// SeedDefaultWebWorker idempotently provisions the minimal public Web Server
// configuration used by install.sh. It only manages resources named
// "http", "default", and "worker", leaving all other user configuration intact.
func (s *Server) SeedDefaultWebWorker() error {
	if err := s.WebServerRepository().SeedDefaultWorkerHost(context.Background()); err != nil {
		return err
	}
	return s.setServiceDesired("web", true)
}

func (s *Server) startWebServer() error {
	if !s.serviceDesiredEnabled("web", true) {
		return nil
	}
	if err := s.validateServiceStart("web"); err != nil {
		s.setServiceError("web", err)
		return nil
	}
	if err := s.applyYabber(s.monitorContext); err != nil {
		log.Printf("Yabber failed to start: %v", err)
		s.setServiceError("web", err)
		return nil
	}
	s.setServiceError("web", nil)
	return nil
}
