// Command ohmyserver runs the server panel: every module in one process,
// under one sign-in, sharing one database, wired together where they
// cooperate.
//
// Each module is an independent project that also runs on its own
// (docker-go, acme, yabber, dns-go, forward-go, ssh-go, cron-go,
// monitor-go, ci-go) and follows the module contract
// (docs/module-contract.md). This command only assembles them: it builds
// each module, gives it the shared database or a store backed by it,
// connects modules through the adapters in packages gateway, dns, cron and
// monitor, and
// mounts them in the shell.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	acmeweb "github.com/lsongdev/acme-go/web"
	ciapi "github.com/lsongdev/ci-go/api"
	ciweb "github.com/lsongdev/ci-go/web"
	"github.com/lsongdev/cron-go/runner"
	cronweb "github.com/lsongdev/cron-go/web"
	dnsconfig "github.com/lsongdev/dns-go/config"
	dnsweb "github.com/lsongdev/dns-go/web"
	dockerweb "github.com/lsongdev/docker-go/web"
	forwardweb "github.com/lsongdev/forward-go/web"
	"github.com/lsongdev/monitor-go/collector"
	monitorweb "github.com/lsongdev/monitor-go/web"
	"github.com/lsongdev/ohmyserver/cron"
	"github.com/lsongdev/ohmyserver/dns"
	"github.com/lsongdev/ohmyserver/gateway"
	"github.com/lsongdev/ohmyserver/home"
	"github.com/lsongdev/ohmyserver/internal/shell"
	"github.com/lsongdev/ohmyserver/internal/stores"
	"github.com/lsongdev/ohmyserver/monitor"
	sshweb "github.com/lsongdev/ssh-go/web"
	"github.com/lsongdev/yabber"
	yabberapi "github.com/lsongdev/yabber/api"
	yabberweb "github.com/lsongdev/yabber/web"
	_ "modernc.org/sqlite"
)

const usage = `usage: ohmyserver [flags]

Serves the OhMyServer panel. Set OHMYSERVER_PASSWORD to require signing in.

Flags:
`

func main() {
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage); flag.PrintDefaults() }
	listen := flag.String("listen", "127.0.0.1:8080", "panel listen address")
	dataDir := flag.String("data", defaultDataDir(), "data directory: the shared database and one subdirectory per module")
	appsHost := flag.String("host", "", "address users reach installed apps at (default: derived from -listen)")
	catalog := flag.String("catalog", dockerweb.DefaultCatalog, `app catalog git repository ("-": use the local copy as is)`)
	gatewayHTTP := flag.String("gateway-http", ":80", "gateway HTTP listener for a new installation")
	gatewayHTTPS := flag.String("gateway-https", ":443", "gateway HTTPS listener for a new installation")
	dnsListen := flag.String("dns-listen", ":53", "DNS listener (UDP and TCP) for a new installation")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(config{
		listen: *listen, dataDir: *dataDir, appsHost: *appsHost, catalog: *catalog,
		gatewayHTTP: *gatewayHTTP, gatewayHTTPS: *gatewayHTTPS, dnsListen: *dnsListen,
	}, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

type config struct {
	listen, dataDir, appsHost, catalog string
	gatewayHTTP, gatewayHTTPS          string
	dnsListen                          string
}

func run(cfg config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dataDir, err := filepath.Abs(cfg.dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}

	// One database for every module (docs/module-contract.md, Storage).
	db, err := stores.Open(filepath.Join(dataDir, "ohmyserver.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	// DNS server: uses SQLite itself, so it takes the database, which also
	// holds its server settings (created from the flags on first run, then
	// edited in the panel). Its TLS listeners serve the certificate
	// manager's certificates; certificateSource is set once that exists,
	// before any listener starts.
	var certificateSource func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	dnsServer, err := dnsweb.New(ctx, db, dnsweb.Options{
		Server: defaultDNS(cfg.dnsListen), DataDir: filepath.Join(dataDir, "dns"), Logger: logger,
		Certificates: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) { return certificateSource(hello) },
	})
	if err != nil {
		return fmt.Errorf("dns: %w", err)
	}

	// Certificate manager: uses SQLite itself, so it takes the database.
	// DNS-01 challenges for zones the DNS server serves authoritatively are
	// published there.
	certs, err := acmeweb.New(ctx, db, acmeweb.Options{Logger: logger, DNSProvider: dns.Challenges{Manager: dnsServer.Manager}})
	if err != nil {
		return fmt.Errorf("certificates: %w", err)
	}
	go certs.Service.RunAutoRenew(ctx, 12*time.Hour)
	certificateSource = dns.Certificates(certs.Service)
	if err := dnsServer.Start(); err != nil {
		// Keep the panel up: listener errors show in the overview and the
		// DNS settings page, where the addresses can be changed.
		logger.Error("some DNS listeners did not start", "err", err)
	}

	// Gateway: keeps its configuration in the database instead of a file,
	// serves the certificate manager's certificates and answers its
	// HTTP-01 challenges.
	gatewayStore, err := stores.NewGatewayConfig(ctx, db)
	if err != nil {
		return err
	}
	gatewayConfig, ok, err := gatewayStore.Load(ctx)
	if err != nil {
		return err
	}
	if !ok {
		gatewayConfig = yabber.Config{
			Listeners: []yabber.Listener{{Name: "http", Address: cfg.gatewayHTTP}, {Name: "https", Address: cfg.gatewayHTTPS, TLS: true}},
		}
		if err := gatewayStore.Save(gatewayConfig); err != nil {
			return err
		}
		logger.Info("created the gateway configuration", "http", cfg.gatewayHTTP, "https", cfg.gatewayHTTPS)
	}
	gatewayDir := filepath.Join(dataDir, "yabber")
	if err := os.MkdirAll(gatewayDir, 0o700); err != nil {
		return err
	}
	gw, err := yabber.New(gatewayConfig,
		yabber.WithLogger(logger), yabber.WithDataDir(gatewayDir),
		yabber.WithCertificateSource(gateway.Certificates(certs.Service)),
		yabber.WithHTTP01Store(gateway.Challenges(certs.Service)),
	)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	defer gw.Close()
	if err := gw.Start(); err != nil {
		// Keep the panel up: listener errors show in the overview and the
		// gateway's own pages, where the configuration can be fixed.
		logger.Error("gateway listeners did not start", "err", err)
	}
	management, err := yabberapi.New(gw, yabberapi.Options{Save: gatewayStore.Save})
	if err != nil {
		return err
	}

	// Docker: install records in the database, apps published through the
	// gateway and, when a DNS zone contains their domain, in DNS.
	dockerApps, err := stores.NewDockerApps(ctx, db)
	if err != nil {
		return err
	}
	appsHost := cfg.appsHost
	if appsHost == "" {
		appsHost = defaultHost(cfg.listen)
	}
	exposer := &gateway.AppExposer{
		Gateway: management, Server: gw, Certs: certs.Service,
		Records: &dns.AppRecords{Manager: dnsServer.Manager, Address: appsHost},
	}
	go exposer.RunReconcile(ctx, time.Minute, logger)
	docker, err := dockerweb.New(ctx, dockerweb.Options{
		DataDir: filepath.Join(dataDir, "docker"), CatalogRepo: cfg.catalog, AppsHost: appsHost, Logger: logger,
		Store: dockerApps, Exposer: exposer,
	})
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}

	// Port forwarder and SSH terminal: both use SQLite themselves, so they
	// take the database.
	forwarder, err := forwardweb.New(ctx, db, forwardweb.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("forward: %w", err)
	}
	terminals, err := sshweb.New(ctx, db, sshweb.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("ssh: %w", err)
	}

	// Scheduler: jobs run on this machine or on the SSH module's hosts.
	scheduler, err := cronweb.New(ctx, db, cronweb.Options{
		Runners: []runner.Runner{cron.SSHHosts{Service: terminals.Service}}, Logger: logger,
	})
	if err != nil {
		return fmt.Errorf("cron: %w", err)
	}

	// Host monitor: this machine and the SSH module's hosts, read over SSH
	// without installing anything on them.
	sshCollector := &monitor.SSHHosts{Service: terminals.Service}
	defer sshCollector.Close() // after the monitor stops (deferred later, runs first)
	monitoring, err := monitorweb.New(ctx, db, monitorweb.Options{
		Collectors: []collector.Collector{sshCollector}, Logger: logger,
	})
	if err != nil {
		return fmt.Errorf("monitor: %w", err)
	}

	// CI: builds run in containers on the Docker daemon the Docker module
	// manages, so built images appear there; webhooks reach it without the
	// panel's sign-in (its manifest lists them as public).
	builds, err := ciweb.New(ctx, db, ciweb.Options{Options: ciapi.Options{Logger: logger}})
	if err != nil {
		return fmt.Errorf("ci: %w", err)
	}

	opts := shell.Options{
		Title: "OhMyServer", Home: "home",
		Titles: map[string]string{"acme": "Certificates", "yabber": "Gateway"},
	}
	if password := os.Getenv("OHMYSERVER_PASSWORD"); password != "" {
		opts.Auth = &shell.Auth{Secret: password}
	} else if !shell.IsLoopback(cfg.listen) {
		logger.Warn("serving without authentication on a non-loopback address; set OHMYSERVER_PASSWORD")
	}
	host, err := shell.New(opts,
		home.New(home.Sources{
			Docker: docker, Certs: certs.Service, Gateway: gw, DNS: dnsServer,
			Forward: forwarder.Service, SSH: terminals.Service, Cron: scheduler.Service, Monitor: monitoring.Service,
			CI: builds.Service,
		}),
		docker, certs, yabberweb.New(management), dnsServer, forwarder, terminals, scheduler, monitoring, builds,
	)
	if err != nil {
		return err
	}
	defer host.Close() // closes the modules after the server stops

	logger.Info("panel listening", "url", "http://"+cfg.listen, "data", dataDir)
	return shell.ListenAndServe(ctx, cfg.listen, host.Handler())
}

// defaultDataDir is ~/.ohmyserver, or ./data when there is no home
// directory (e.g. a service user without one).
func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./data"
	}
	return filepath.Join(home, ".ohmyserver")
}

// defaultDNS is a new installation's DNS server: a forwarding resolver for
// the local network, with a cache and encrypted upstreams first.
func defaultDNS(listen string) dnsconfig.Server {
	return dnsconfig.Server{
		Mode:    dnsconfig.ModeForwarding,
		Listens: []dnsconfig.ListenSpec{{Type: "udp", Addr: listen}, {Type: "tcp", Addr: listen}},
		Upstreams: []dnsconfig.UpstreamSpec{
			{Type: "doh", Addr: "https://doh.pub/dns-query"},
			{Type: "dot", Addr: "1.1.1.1:853"},
			{Type: "udp", Addr: "8.8.8.8:53"},
		},
		Cache: dnsconfig.CacheSpec{Enabled: true},
	}
}

// defaultHost picks the address apps are advertised at: the listen host, or
// the primary outbound IP when listening on all interfaces.
func defaultHost(listen string) string {
	h, _, err := net.SplitHostPort(listen)
	if err != nil {
		return "localhost"
	}
	switch h {
	case "127.0.0.1", "::1", "localhost":
		return "localhost"
	case "", "0.0.0.0", "::":
		// A UDP "connection" sends nothing; it just selects the route.
		if conn, err := net.Dial("udp", "192.0.2.1:9"); err == nil {
			defer conn.Close()
			return conn.LocalAddr().(*net.UDPAddr).IP.String()
		}
		return "localhost"
	}
	return h
}
