package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/config"
	acmemodule "github.com/lsongdev/ohmyserver/modules/acme"
	acmeweb "github.com/lsongdev/ohmyserver/modules/acme/web"
	cronweb "github.com/lsongdev/ohmyserver/modules/cron/web"
	dhcpweb "github.com/lsongdev/ohmyserver/modules/dhcp/web"
	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
	dockermodule "github.com/lsongdev/ohmyserver/modules/docker"
	esxiweb "github.com/lsongdev/ohmyserver/modules/esxi/web"
	forwardweb "github.com/lsongdev/ohmyserver/modules/forward/web"
	idracweb "github.com/lsongdev/ohmyserver/modules/idrac/web"
	ipmiweb "github.com/lsongdev/ohmyserver/modules/ipmi/web"
	monitorweb "github.com/lsongdev/ohmyserver/modules/monitor/web"
	ntpweb "github.com/lsongdev/ohmyserver/modules/ntp/web"
	projectsweb "github.com/lsongdev/ohmyserver/modules/projects/web"
	sshweb "github.com/lsongdev/ohmyserver/modules/ssh/web"
	uptimeweb "github.com/lsongdev/ohmyserver/modules/uptime/web"
	webruntime "github.com/lsongdev/ohmyserver/modules/webserver/web"
)

type Server struct {
	db                           *sql.DB
	acmeRepository               *acmeweb.Repository
	acmeEngine                   *acmemodule.Engine
	config                       *config.Config
	docker                       *dockermodule.DockerClient
	dockerService                *dockermodule.LocalService
	dockerServiceOnce            sync.Once
	deployRuntime                projectsweb.DeploymentRuntime
	forwardService               *forwardweb.LocalService
	forwardServiceOnce           sync.Once
	webRuntime                   *webruntime.Runtime
	webRuntimeMu                 sync.Mutex
	monitorContext               context.Context
	monitorCancel                context.CancelFunc
	monitorCollection            *monitorweb.CollectionService
	dnsService                   *dnsweb.LocalService
	ntpModule                    *ntpweb.LocalModule
	ntpModuleOnce                sync.Once
	uptimeRepository             *uptimeweb.Repository
	cronModule                   *cronweb.LocalModule
	sshModule                    *sshweb.LocalModule
	sshModuleOnce                sync.Once
	dhcpModule                   *dhcpweb.LocalModule
	dhcpModuleOnce               sync.Once
	dhcpInitialized              bool
	esxiModule                   *esxiweb.LocalModule
	esxiModuleOnce               sync.Once
	idracModule                  *idracweb.LocalModule
	idracModuleOnce              sync.Once
	ipmiModule                   *ipmiweb.LocalModule
	ipmiModuleOnce               sync.Once
	projectDeployments           *projectsweb.DeploymentManager
	projectDeploymentManagerOnce sync.Once
	startedAt                    time.Time
	closeOnce                    sync.Once
	closeErr                     error
}

func NewServer(cfg *config.Config) (server *Server, err error) {
	d, err := dockermodule.NewDockerClient(cfg.Docker.RegistryMirror)
	if err != nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.Database.Path)
	if err != nil {
		return nil, err
	}

	monitorContext, monitorCancel := context.WithCancel(context.Background())
	server = &Server{
		db:               db,
		acmeRepository:   acmeweb.NewRepository(db),
		uptimeRepository: uptimeweb.NewRepository(db),
		docker:           d,
		config:           cfg,
		monitorContext:   monitorContext,
		monitorCancel:    monitorCancel,
		startedAt:        time.Now(),
	}
	if err := server.initSchema(); err != nil {
		monitorCancel()
		db.Close()
		return nil, err
	}
	if err := server.ensureServicePreferences(); err != nil {
		monitorCancel()
		db.Close()
		return nil, err
	}
	server.acmeEngine = acmemodule.NewEngine(monitorContext, server.acmeRepository, acmeDNSChallengeAdapter{server: server}, server.onACMECertificateUpdated)
	server.cronModule = cronweb.NewLocalModule(monitorContext, db, nil)
	if err := server.cronModule.Start(monitorContext); err != nil {
		_ = server.acmeEngine.Close()
		_ = server.cronModule.Close()
		monitorCancel()
		_ = db.Close()
		return nil, fmt.Errorf("start cron module: %w", err)
	}

	// Run init hooks (post-schema initialization)
	server.startServices()
	return
}

// Close closes the database connection and stops all listeners
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		if s.acmeEngine != nil {
			_ = s.acmeEngine.Close()
		}
		if s.cronModule != nil {
			_ = s.cronModule.Close()
		}
		if s.ntpModule != nil {
			_ = s.ntpModule.Close()
		}
		if s.monitorCancel != nil {
			s.monitorCancel()
		}
		if s.projectDeployments != nil {
			s.projectDeployments.Close()
		}
		var collectionErr error
		if s.monitorCollection != nil {
			collectionErr = s.monitorCollection.Close()
		}
		s.closeErr = errors.Join(s.stopServices(), collectionErr, s.docker.Close(), s.db.Close())
	})
	return s.closeErr
}
