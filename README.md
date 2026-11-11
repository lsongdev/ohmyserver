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
| DHCP server (leases, reservations) | dhcp-go | `dhcp-go -config …` |

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
| Network (interfaces and settings, routes, ports, diagnostics) | [modules/netmgr](modules/netmgr) | `netmgr` |
| Disks (capacity, RAID, SMART health) | [modules/diskmgr](modules/diskmgr) | `diskmgr` |
| Samba shares, settings and users (smb.conf) | [modules/smbmgr](modules/smbmgr) | `smbmgr` |
| ZFS pools, datasets, snapshots and schedules | [modules/zfsmgr](modules/zfsmgr) | `zfsmgr` |
| Notifications: what modules report and what is still wrong | [modules/notify](modules/notify) | `notify-go` |
| Files (browse, edit, upload, download, move, copy) | [modules/filemgr](modules/filemgr) | `filemgr` |
| Domains at DNS providers (Cloudflare), and DNS-01 for certificates | [modules/domains](modules/domains) | `domains` |

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
  (docker-go's install records, yabber's configuration, forward-go's
  rules, dhcp-go's settings, reservations and leases) in the shared
  database, by implementing their store interfaces.
- `gateway` connects modules through interfaces they define: the gateway
  serves acme-go's certificates and answers its HTTP-01 challenges, and
  docker-go apps with a domain become gateway sites with a certificate.
- `dns` connects dns-go the same way: DNS-01 challenges for zones dns-go
  serves authoritatively are published there automatically, and for zones
  a domains account (Cloudflare) holds, there (other names stay manual), DoT/DoH listeners with `tls: true` serve acme-go's
  certificates, and an app domain inside a zone dns-go manages gets an
  address record pointing at `host` (config.yaml).
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
- `modules/netmgr` shows interfaces, routes, DNS servers and listening ports,
  runs diagnostics, and changes interface settings through NetworkManager or
  systemd-networkd. A change is on trial: it reverts by itself unless it is
  kept within two minutes, even if the panel crashes meanwhile. It is left
  out outside Linux.
- `modules/diskmgr` shows file system capacity, software RAID arrays and each
  disk's SMART health (through smartctl-go), without waking sleeping disks.
- `modules/smbmgr` edits Samba's smb.conf (shares, server settings, or the
  file as text) and its users; testparm checks every change before it
  replaces the file, and smbd reloads it. It is left out without Samba.
- `modules/zfsmgr` takes, compares, deletes and rolls back to ZFS snapshots,
  and applies schedules with retention stored on the datasets, from a job in
  the job scheduler; retention only removes the snapshots schedules took. It
  is left out without ZFS.
- `modules/notify` records what modules report and tracks the problems still
  open, shown on the overview, and sends them to chat services by each channel's
  rule, with an outbox, retries and rate limits.
- `modules/filemgr` manages the server's files from `/`; system top-level
  directories cannot be deleted or moved.

The SSH and cron modules run commands on this machine and every saved
host, so set a `password` whenever the panel listens beyond
loopback.
- `cmd/ohmyserver` opens the shared SQLite database (`<data>/ohmyserver.db`),
  builds every module and mounts them.

Settings come from `<dir>/config.yaml`, where `<dir>` is `-d` (default
`~/.ohmyserver`); without the file every setting takes its default.
[`config.yaml.example`](config.yaml.example) documents each one: the
panel's `listen` address, `data` directory (`<dir>` itself by default),
sign-in `password` (`OHMYSERVER_PASSWORD` overrides it), apps `host`, the
first-run gateway and DNS listeners, and the ACME timeouts and retries.
For development, copy it to `config.yaml` in the checkout (git-ignored):

```sh
make deps     # npm ci in each UI: its browser dependencies
cp config.yaml.example config.yaml   # then set data: ./data
go run ./cmd/ohmyserver -d .
```

Each UI, the shell's (`internal/shell/ui/static`) and every module's
(`modules/*/web/ui` or `web/ui` in a sibling checkout), declares its browser
dependencies in its own `package.json`: @lsongdev/stylesheets, @lsongdev/scripts (with Preact and
htm) and, for SSH, xterm.js. Go embeds them from that UI's `node_modules`,
which is not committed, so run `make deps` (or `make build`, `make test`)
before any `go build`, `go run` or `go test`; without it the shell refuses
to start and says so.

On Linux with systemd, install or upgrade from the latest GitHub release:

```sh
curl -fsSLo install.sh https://raw.githubusercontent.com/lsongdev/ohmyserver/master/install.sh &&
  sudo sh install.sh
```

The installer verifies the binary and [systemd unit](contrib/systemd/ohmyserver.service)
against the release's SHA-256 checksums, then enables and restarts the service.
It supports AMD64, ARM64, ARMv7 and RISC-V 64 and preserves existing configuration
and data. The panel defaults to `http://127.0.0.1:8080`. From another machine,
forward the port with `ssh -L 8080:127.0.0.1:8080 user@server` and open that
address in your browser. To configure the service, use
[`config.yaml.example`](config.yaml.example) as a reference for
`/var/lib/ohmyserver/config.yaml`. Set a password before exposing the panel.

The gateway listens on :80 and :443 by default; `gateway.http` and
`gateway.https` choose other addresses for a new installation, after which
the configuration is edited in the panel. The DNS server is off until enabled
under DNS → Settings. Its initial configuration is a forwarding resolver on
:53 (UDP and TCP, `dns.listen` for a new installation). Changes to its
listeners, upstreams and mode take effect at once. A listener that cannot bind (a port
taken, or :53 without the privilege) shows in the overview instead of
stopping the panel.

ACME network and recovery settings (the `acme` section) are passed to
acme-go. Defaults are a 60-second request timeout, 30-second TLS handshake
timeout, four safe-read attempts and four issuance attempts with backoff
starting at one minute; `validation_timeout` (5m) bounds each
authorization wait and `order_timeout` (10m) finalization and certificate
download. An attempt limit of 1 disables retries at that layer. Unfinished issuances
resume from their saved order and key after restart; the certificate panel
shows attempt counts and the next retry time.

Development uses `go.work` with sibling checkouts of the projects
(`../docker-go`, `../acme-go`, `../yabber`, `../dns-go`, `../forward-go`)
and of the smartctl-go library (`../smartctl-go`).
