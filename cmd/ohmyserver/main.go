// Command ohmyserver runs the server panel: every module in one process,
// under one sign-in, sharing one database, wired together where they
// cooperate.
//
// Each module is an independent project that also runs on its own
// (docker-go, acme, yabber) and follows the module contract
// (docs/module-contract.md). This command only assembles them: it builds
// each module, gives it the shared database or a store backed by it,
// connects modules through the adapters in package gateway, and mounts them
// in the shell.
package main

import (
	"context"
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
	dockerweb "github.com/lsongdev/docker-go/web"
	"github.com/lsongdev/ohmyserver/gateway"
	"github.com/lsongdev/ohmyserver/home"
	"github.com/lsongdev/ohmyserver/internal/shell"
	"github.com/lsongdev/ohmyserver/internal/stores"
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
	dataDir := flag.String("data", "./data", "data directory: the shared database and one subdirectory per module")
	appsHost := flag.String("host", "", "address users reach installed apps at (default: derived from -listen)")
	catalog := flag.String("catalog", dockerweb.DefaultCatalog, `app catalog git repository ("-": use the local copy as is)`)
	gatewayHTTP := flag.String("gateway-http", ":80", "gateway HTTP listener for a new installation")
	gatewayHTTPS := flag.String("gateway-https", ":443", "gateway HTTPS listener for a new installation")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(config{
		listen: *listen, dataDir: *dataDir, appsHost: *appsHost, catalog: *catalog,
		gatewayHTTP: *gatewayHTTP, gatewayHTTPS: *gatewayHTTPS,
	}, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

type config struct {
	listen, dataDir, appsHost, catalog string
	gatewayHTTP, gatewayHTTPS          string
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

	// Certificate manager: uses SQLite itself, so it takes the database.
	certs, err := acmeweb.New(ctx, db, acmeweb.Options{Logger: logger})
	if err != nil {
		return fmt.Errorf("certificates: %w", err)
	}
	go certs.Service.RunAutoRenew(ctx, 12*time.Hour)

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
	// gateway.
	dockerApps, err := stores.NewDockerApps(ctx, db)
	if err != nil {
		return err
	}
	appsHost := cfg.appsHost
	if appsHost == "" {
		appsHost = defaultHost(cfg.listen)
	}
	docker, err := dockerweb.New(ctx, dockerweb.Options{
		DataDir: filepath.Join(dataDir, "docker"), CatalogRepo: cfg.catalog, AppsHost: appsHost, Logger: logger,
		Store: dockerApps, Exposer: &gateway.AppExposer{Gateway: management, Certs: certs.Service},
	})
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}

	opts := shell.Options{
		Title: "OhMyServer", Home: "home",
		Titles: map[string]string{"acme": "证书", "yabber": "网关"},
	}
	if password := os.Getenv("OHMYSERVER_PASSWORD"); password != "" {
		opts.Auth = &shell.Auth{Secret: password}
	} else if !shell.IsLoopback(cfg.listen) {
		logger.Warn("serving without authentication on a non-loopback address; set OHMYSERVER_PASSWORD")
	}
	host, err := shell.New(opts,
		home.New(home.Sources{Docker: docker, Certs: certs.Service, Gateway: gw}),
		docker, certs, yabberweb.New(management),
	)
	if err != nil {
		return err
	}
	defer host.Close() // closes docker and certs after the server stops

	logger.Info("panel listening", "url", "http://"+cfg.listen, "data", dataDir)
	return shell.ListenAndServe(ctx, cfg.listen, host.Handler())
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
