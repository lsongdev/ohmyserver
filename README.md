# OhMyServer

A server panel assembled from modules. Mature modules are projects of their
own that also run on their own:

| Module | Project | Standalone |
|---|---|---|
| Docker (containers, compose, app store) | docker-go | `docker-go` |
| Certificates (ACME) | acme-go | `acme serve` |
| Gateway (reverse proxy, TLS) | yabber | `yabber run -admin …` |
| DNS (resolver, filtering, zones) | dns-go | `dns-go -config …` |
| Port forwarding (TCP/UDP) | forward-go | `forward-go` |

Modules that are still small and change with the panel live in
[`modules/`](modules) until they are mature enough to stand on their own:

| Module | Directory | Standalone |
|---|---|---|
| SSH hosts and web terminal | [modules/ssh](modules/ssh) | `ssh-go` |
| Scheduled jobs (cron) | [modules/cron](modules/cron) | `cron-go` |
| Host monitoring | [modules/monitor](modules/monitor) | `monitor-go` |
| CI (builds and tests in Docker) | [modules/ci](modules/ci) | `ci-go` |
| Packages (dnf, apt, installing Docker) | [modules/pkgmgr](modules/pkgmgr) | `pkgmgr` |
| Services (systemd) and logs (journal) | [modules/svcmgr](modules/svcmgr) | `svcmgr` |
| Disks (capacity, RAID, SMART health) | [modules/diskmgr](modules/diskmgr) | `diskmgr` |

Every module follows the [module contract](docs/module-contract.md): an API
handler, assets with a `manifest.json`, and a `mount(el, ctx)` entry point.
Modules never import each other or the panel; the ones in `modules/` import
nothing else from this repository either, so each directory can move out
into a project of its own as it is.

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
- `cron` lets scheduled jobs run on the SSH module's saved hosts as well as on this
  machine, with the hosts' stored credentials and pinned host keys.
- `monitor` lets the monitor module read the SSH module's saved Linux hosts over SSH, with
  nothing installed on them (one kept connection per host); the overview
  shows this machine's CPU, memory and disk.
- CI builds on the Docker daemon the Docker module manages, so built
  images appear under Images; its webhook path is public in its manifest,
  so git hosts reach it without the panel's sign-in.
- `home` is the panel's own overview module.
- `modules/pkgmgr` drives the system's package manager (dnf or apt) and installs
  Docker for the modules that need it; the overview and the Docker module
  link there when Docker is missing.
- `modules/svcmgr` manages systemd services and reads the journal; it never stops
  or disables the service running the panel.
- `modules/diskmgr` shows file system capacity, software RAID arrays and each
  disk's SMART health (through smartctl-go), without waking sleeping disks.

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

Development uses `go.work` with sibling checkouts of the projects
(`../docker-go`, `../acme-go`, `../yabber`, `../dns-go`, `../forward-go`)
and of the smartctl-go library (`../smartctl-go`).
