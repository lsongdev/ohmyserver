# OhMyServer

A server panel assembled from independent modules. Each module is its own
project that also runs on its own:

| Module | Project | Standalone |
|---|---|---|
| Docker (containers, compose, app store) | docker-go | `docker-go` |
| Certificates (ACME) | acme-go | `acme serve` |
| Gateway (reverse proxy, TLS) | yabber | `yabber run -admin …` |
| DNS (resolver, filtering, zones) | dns-go | `dns-go -config …` |
| Port forwarding (TCP/UDP) | forward-go | `forward-go` |
| SSH hosts and web terminal | ssh-go | `ssh-go` |
| Scheduled jobs (cron) | cron-go | `cron-go` |
| Host monitoring | monitor-go | `monitor-go` |
| CI (builds and tests in Docker) | ci-go | `ci-go` |
| Packages (dnf, apt, installing Docker) | built in: [pkgmgr](pkgmgr) | `pkgmgr` |
| Services (systemd) and logs (journal) | built in: [svcmgr](svcmgr) | `svcmgr` |

Modules follow the [module contract](docs/module-contract.md): an API
handler, assets with a `manifest.json`, and a `mount(el, ctx)` entry point.
They never import this repository or each other. A module not yet mature
enough for a project of its own lives in a directory here (`pkgmgr`, `svcmgr`),
follows the same contract, and imports nothing else from this repository.

This repository holds the assembly:

- `internal/shell` mounts contract modules under one sign-in: it reads
  each manifest, serves the API at `/m/<id>/api/` and the assets at
  `/m/<id>/ui/`, and renders each module into its own shadow root, passing
  the theme through the @lsongdev/stylesheets CSS variables.
- `internal/stores` keeps the state of modules that use files on their own
  (docker-go's install records, yabber's configuration) in the shared
  database, by implementing their store interfaces.
- `gateway` connects modules through interfaces they define: the gateway
  serves acme-go's certificates and answers its HTTP-01 challenges, and
  docker-go apps with a domain become gateway sites with a certificate.
- `dns` connects dns-go the same way: DNS-01 challenges for zones dns-go
  serves authoritatively are published there automatically (other names
  stay manual), DoT/DoH listeners with `tls: true` serve acme-go's
  certificates, and an app domain inside a zone dns-go manages gets an
  address record pointing at `-host`.
- `cron` lets cron-go jobs run on ssh-go's saved hosts as well as on this
  machine, with the hosts' stored credentials and pinned host keys.
- `monitor` lets monitor-go read ssh-go's saved Linux hosts over SSH, with
  nothing installed on them (one kept connection per host); the overview
  shows this machine's CPU, memory and disk.
- ci-go builds on the Docker daemon the Docker module manages, so built
  images appear under Images; its webhook path is public in its manifest,
  so git hosts reach it without the panel's sign-in.
- `home` is the panel's own overview module.
- `pkgmgr` drives the system's package manager (dnf or apt) and installs
  Docker for the modules that need it; the overview and the Docker module
  link there when Docker is missing.
- `svcmgr` manages systemd services and reads the journal; it never stops
  or disables the service running the panel.

The SSH and cron modules run commands on this machine and every saved
host, so set `OHMYSERVER_PASSWORD` whenever the panel listens beyond
loopback.
- `cmd/ohmyserver` opens the shared SQLite database (`<data>/ohmyserver.db`),
  builds every module and mounts them. The data directory is
  `~/.ohmyserver` unless `-data` says otherwise.

```sh
OHMYSERVER_PASSWORD=… go run ./cmd/ohmyserver -listen 127.0.0.1:8080
```

On a server it runs as a systemd service:
[`contrib/systemd/ohmyserver.service`](contrib/systemd/ohmyserver.service)
explains installing it, with the password and the machine's own flags in
`/etc/ohmyserver.env`.

The gateway listens on :80 and :443 by default; `-gateway-http` and
`-gateway-https` choose other addresses for a new installation, after which
the configuration is edited in the panel. Likewise the DNS server starts as
a forwarding resolver on :53 (UDP and TCP, `-dns-listen` for a new
installation); its listeners, upstreams and mode are then changed under
DNS → Settings and take effect at once. A listener that cannot bind (a port
taken, or :53 without the privilege) shows in the overview instead of
stopping the panel.

ACME network and recovery settings are passed to acme-go. Defaults are a
60-second request timeout, 30-second TLS handshake timeout, four safe-read
attempts and four issuance attempts with backoff starting at one minute.
Override them with `-acme-request-timeout`, `-acme-tls-timeout`,
`-acme-request-attempts`, `-acme-issuance-attempts` and `-acme-retry-delay`.
`-acme-validation-timeout` (5m) bounds each authorization wait;
`-acme-order-timeout` (10m) bounds finalization and certificate download.
An attempt limit of 1 disables retries at that layer. Unfinished issuances
resume from their saved order and key after restart; the certificate panel
shows attempt counts and the next retry time.

Development uses `go.work` with sibling checkouts of the modules
(`../docker-go`, `../acme-go`, `../yabber`, `../dns-go`, `../forward-go`,
`../ssh-go`, `../cron-go`, `../monitor-go`, `../ci-go`). The remaining modules of the
previous monolith (on `main`) are to be extracted the same way.
