# OhMyServer

A server panel assembled from independent modules. Each module is its own
project that also runs on its own:

| Module | Project | Standalone |
|---|---|---|
| Docker (containers, compose, app store) | docker-go | `docker-go` |
| Certificates (ACME) | acme-go | `acme serve` |
| Gateway (reverse proxy, TLS) | yabber | `yabber run -admin …` |
| DNS (resolver, filtering, zones) | dns-go | `dns-go -config …` |

Modules follow the [module contract](docs/module-contract.md): an API
handler, assets with a `manifest.json`, and a `mount(el, ctx)` entry point.
They never import this repository or each other.

This repository holds the assembly:

- `internal/shell` mounts contract modules under one sign-in: it reads
  each manifest, serves the API at `/m/<id>/api/` and the assets at
  `/m/<id>/ui/`, and renders each module into its own shadow root, passing
  the theme through `--panel-*` CSS variables.
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
- `home` is the panel's own overview module.
- `cmd/ohmyserver` opens the shared SQLite database (`<data>/ohmyserver.db`),
  builds every module and mounts them.

```sh
OHMYSERVER_PASSWORD=… go run ./cmd/ohmyserver -listen 127.0.0.1:8080 -data ./data
```

The gateway listens on :80 and :443 by default; `-gateway-http` and
`-gateway-https` choose other addresses for a new installation, after which
the configuration is edited in the panel. Likewise the DNS server starts as
a forwarding resolver on :53 (UDP and TCP, `-dns-listen` for a new
installation); its listeners, upstreams and mode are then changed under
DNS → 服务设置 and take effect at once. A listener that cannot bind (a port
taken, or :53 without the privilege) shows in the overview instead of
stopping the panel.

Development uses `go.work` with sibling checkouts of the modules
(`../docker-go`, `../acme-go`, `../yabber`, `../dns-go`). The remaining modules of the
previous monolith (on `main`) are to be extracted the same way.
