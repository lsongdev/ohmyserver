// Command ohmyserver runs the server panel: every module in one process,
// under one sign-in, sharing one database, wired together where they
// cooperate.
//
// Each module is an independent project that also runs on its own
// (docker-go, acme, yabber, dns-go, dhcp-go, forward-go, ssh-go, cron-go,
// monitor-go, ci-go) and follows the module contract
// (docs/module-contract.md). This command only assembles them: it builds
// each module, gives it the shared database or a store backed by it,
// connects modules through the adapters in packages gateway, dns, cron and
// monitor, and
// mounts them in the shell, together with the modules built in here (the
// overview, domains, packages, services, disks, Samba, ZFS snapshots and files).
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

	"github.com/lsongdev/acme-go/acme"
	acmeapi "github.com/lsongdev/acme-go/api"
	acmeweb "github.com/lsongdev/acme-go/web"
	dhcpweb "github.com/lsongdev/dhcp-go/web"
	dnsconfig "github.com/lsongdev/dns-go/config"
	dnsweb "github.com/lsongdev/dns-go/web"
	dockerweb "github.com/lsongdev/docker-go/web"
	forwardweb "github.com/lsongdev/forward-go/web"
	"github.com/lsongdev/ohmyserver/config"
	"github.com/lsongdev/ohmyserver/cron"
	"github.com/lsongdev/ohmyserver/dns"
	"github.com/lsongdev/ohmyserver/gateway"
	"github.com/lsongdev/ohmyserver/home"
	"github.com/lsongdev/ohmyserver/internal/shell"
	"github.com/lsongdev/ohmyserver/internal/stores"
	ciapi "github.com/lsongdev/ohmyserver/modules/ci/api"
	ciweb "github.com/lsongdev/ohmyserver/modules/ci/web"
	"github.com/lsongdev/ohmyserver/modules/cron/runner"
	cronweb "github.com/lsongdev/ohmyserver/modules/cron/web"
	diskmgr "github.com/lsongdev/ohmyserver/modules/diskmgr/web"
	domainsweb "github.com/lsongdev/ohmyserver/modules/domains/web"
	filemgr "github.com/lsongdev/ohmyserver/modules/filemgr/web"
	"github.com/lsongdev/ohmyserver/modules/monitor/collector"
	monitorweb "github.com/lsongdev/ohmyserver/modules/monitor/web"
	notifyweb "github.com/lsongdev/ohmyserver/modules/notify/web"
	pkgmgr "github.com/lsongdev/ohmyserver/modules/pkgmgr/web"
	smbmgr "github.com/lsongdev/ohmyserver/modules/smbmgr/web"
	sshweb "github.com/lsongdev/ohmyserver/modules/ssh/web"
	svcmgr "github.com/lsongdev/ohmyserver/modules/svcmgr/web"
	zfsmgr "github.com/lsongdev/ohmyserver/modules/zfsmgr/web"
	"github.com/lsongdev/ohmyserver/monitor"
	"github.com/lsongdev/ohmyserver/notify"
	"github.com/lsongdev/yabber"
	yabberapi "github.com/lsongdev/yabber/api"
	yabberweb "github.com/lsongdev/yabber/web"
	_ "modernc.org/sqlite"
)

// version is set at build time: -ldflags "-X main.version=2026.10.28".
var version = "dev"

const usage = `usage: ohmyserver [-d dir]

Serves the OhMyServer panel with the settings in <dir>/config.yaml (see
config.yaml.example); without that file every setting takes its default.
Set OHMYSERVER_PASSWORD to require signing in instead of setting password.

Flags:
`

func main() {
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage); flag.PrintDefaults() }
	dir := flag.String("d", defaultDir(), "configuration directory, holding config.yaml and, unless it says otherwise, the data")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	cfg, err := config.LoadConfig(*dir)
	if err == nil {
		err = run(cfg, logger)
	}
	if err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dataDir := cfg.Data
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
		Server: defaultDNS(cfg.DNS.Listen), DataDir: filepath.Join(dataDir, "dns"), Logger: logger,
		Certificates: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) { return certificateSource(hello) },
	})
	if err != nil {
		return fmt.Errorf("dns: %w", err)
	}

	// Domains: DNS provider accounts (Cloudflare, …) and the zones they
	// hold; uses SQLite itself, so it takes the database.
	domains, err := domainsweb.New(ctx, db, domainsweb.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("domains: %w", err)
	}

	// Certificate manager: uses SQLite itself, so it takes the database.
	// DNS-01 challenges are published automatically in the zones the DNS
	// server serves authoritatively, or else in a provider account's.
	acmeOptions := acmeapi.Options{
		HTTPClient:        acme.NewHTTPClient(acme.HTTPOptions{Timeout: cfg.ACME.RequestTimeout, TLSHandshakeTimeout: cfg.ACME.TLSTimeout}),
		RequestRetry:      acme.RetryPolicy{MaxAttempts: cfg.ACME.RequestAttempts},
		IssuanceRetry:     acme.RetryPolicy{MaxAttempts: cfg.ACME.IssuanceAttempts, InitialDelay: cfg.ACME.RetryDelay},
		ValidationTimeout: cfg.ACME.ValidationTimeout, OrderTimeout: cfg.ACME.OrderTimeout,
	}
	acmeOptions.Logger = logger
	acmeOptions.DNSProvider = dns.Sources{dns.Challenges{Manager: dnsServer.Manager}, domains.Challenges()}
	certs, err := acmeweb.New(ctx, db, acmeOptions)
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
			Listeners: []yabber.Listener{{Name: "http", Address: cfg.Gateway.HTTP}, {Name: "https", Address: cfg.Gateway.HTTPS, TLS: true}},
		}
		if err := gatewayStore.Save(gatewayConfig); err != nil {
			return err
		}
		logger.Info("created the gateway configuration", "http", cfg.Gateway.HTTP, "https", cfg.Gateway.HTTPS)
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
	appsHost := cfg.Host
	if appsHost == "" {
		appsHost = defaultHost(cfg.Listen)
	}
	exposer := &gateway.AppExposer{
		Gateway: management, Server: gw, Certs: certs.Service,
		Records: &dns.AppRecords{Manager: dnsServer.Manager, Address: appsHost},
	}
	go exposer.RunReconcile(ctx, time.Minute, logger)
	docker, err := dockerweb.New(ctx, dockerweb.Options{
		DataDir: filepath.Join(dataDir, "docker"), CatalogRepo: cfg.Catalog, AppsHost: appsHost, Logger: logger,
		Store: dockerApps, Exposer: exposer,
	})
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}

	// Port forwarder: rules in the database instead of its YAML file.
	// SSH terminal: uses SQLite itself, so it takes the database.
	forwardRules, err := stores.NewForwardRules(ctx, db)
	if err != nil {
		return err
	}
	forwarder, err := forwardweb.NewWithStore(ctx, forwardRules, forwardweb.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("forward: %w", err)
	}
	terminals, err := sshweb.New(ctx, db, sshweb.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("ssh: %w", err)
	}

	// Notifications: what modules report, and what is still wrong.
	notifications, err := notifyweb.New(ctx, db, notifyweb.Options{Logger: logger, Unauthenticated: cfg.Password == ""})
	if err != nil {
		return fmt.Errorf("notifications: %w", err)
	}

	// ZFS: pools, datasets and snapshots, and the schedules stored on the
	// datasets, which a job below applies.
	snapshotJob := &cron.ZFSJob{}
	snapshots := zfsmgr.New(zfsmgr.Options{Logger: logger, Trigger: snapshotJob, Notifier: notify.ZFS{Service: notifications.Service}})

	// Scheduler: jobs run on this machine or on the SSH module's hosts.
	runners := []runner.Runner{cron.SSHHosts{Service: terminals.Service}}
	if snapshots.API.Supported() {
		runners = append(runners, cron.ZFSRunner{Service: snapshots.API})
	}
	scheduler, err := cronweb.New(ctx, db, cronweb.Options{Runners: runners, Logger: logger})
	if err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	snapshotJob.Cron = scheduler.Service
	if snapshots.API.Supported() {
		// Not fatal: the panel works without it, and the ZFS pages say so.
		if err := snapshotJob.Ensure(ctx); err != nil {
			logger.Warn("zfs: creating the snapshot job", "err", err)
		}
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

	// Packages: the system's package manager (dnf or apt), and installing
	// Docker for the modules that need it.
	packages := pkgmgr.New(pkgmgr.Options{Logger: logger})
	// Services: systemd services and the journal.
	services := svcmgr.New(svcmgr.Options{Logger: logger})
	// Disks: capacity, RAID arrays and SMART health.
	disks := diskmgr.New(diskmgr.Options{Logger: logger})
	// Samba: shares, server settings and users in smb.conf, and who is
	// connected.
	samba := smbmgr.New(smbmgr.Options{Logger: logger})
	// Files: browse and manage the server's files, from /.
	fileManager, err := filemgr.New(filemgr.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("files: %w", err)
	}

	// DHCP server: off until it is turned on in its settings, since a
	// second DHCP server on a network hands out conflicting addresses.
	dhcpState, err := stores.NewDHCP(ctx, db)
	if err != nil {
		return err
	}
	dhcpServer, err := dhcpweb.New(ctx, dhcpState, dhcpweb.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("dhcp: %w", err)
	}

	opts := shell.Options{
		Title: "OhMyServer", Home: "home",
		Titles: map[string]string{"acme": "Certificates", "yabber": "Gateway"},
		// Named for what it does, not the project that does it.
		IDs: map[string]string{"yabber": "gateway"},
	}
	if cfg.Password != "" {
		opts.Auth = &shell.Auth{Secret: cfg.Password}
	} else if !shell.IsLoopback(cfg.Listen) {
		logger.Warn("serving without authentication on a non-loopback address; set a password in config.yaml")
	}
	modules := []shell.Module{
		home.New(home.Sources{
			Docker: docker, Certs: certs.Service, Gateway: gw, DNS: dnsServer,
			Forward: forwarder.Service, SSH: terminals.Service, Cron: scheduler.Service, Monitor: monitoring.Service,
			CI: builds.Service, Notifications: notifications.Service,
		}),
		notifications, monitoring, docker, certs, yabberweb.New(management), dnsServer, domains, dhcpServer, forwarder, terminals, scheduler, builds, packages,
	}
	// Services manages systemd: without it (macOS, for one) the module has
	// nothing to show, so it is left out of the panel.
	if services.API.Supported() {
		modules = append(modules, services)
	}
	modules = append(modules, disks)
	// Likewise Samba, without smbd and testparm.
	if samba.API.Supported() {
		modules = append(modules, samba)
	}
	// And ZFS snapshots, without zfs and zpool.
	if snapshots.API.Supported() {
		modules = append(modules, snapshots)
	}
	modules = append(modules, fileManager)
	host, err := shell.New(opts, modules...)
	if err != nil {
		return err
	}
	defer host.Close() // closes the modules after the server stops

	logger.Info("panel listening", "url", "http://"+cfg.Listen, "data", dataDir, "version", version)
	return shell.ListenAndServe(ctx, cfg.Listen, host.Handler())
}

// defaultDir is ~/.ohmyserver, or ./data when there is no home directory
// (e.g. a service user without one).
func defaultDir() string {
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
		// Off until turned on in its settings: a resolver on :53 often
		// collides with the system's own, and is rarely wanted at once.
		Disabled: true,
		Mode:     dnsconfig.ModeForwarding,
		Listens:  []dnsconfig.ListenSpec{{Type: "udp", Addr: listen}, {Type: "tcp", Addr: listen}},
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
