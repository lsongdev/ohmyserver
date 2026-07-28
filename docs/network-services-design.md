# Network Services

## Namespaces and ownership

| Route | Responsibility | Meaning of “listener” |
| --- | --- | --- |
| `/services` | Runtime coordination | Starts, stops, reloads, and reports service state |
| `/dns` | DNS configuration | DNS protocol bindings are configured in DNS settings |
| `/web` | Web Server configuration | `/web/listeners/*` binds HTTP or HTTPS sockets to virtual hosts |
| `/proxy` | Layer 4 forwarding | Each proxy accepts TCP or UDP traffic and forwards it upstream |

“Listener” is an implementation object within a service, not a top-level
product area. The old top-level `/listeners/*` routes permanently redirect to
their matching `/proxy/*` routes. HTTP/HTTPS listeners remain under `/web`
because they depend on Web Server hosts, routes, certificates, and middleware.

The persisted `tcp_listeners` table and `TCPListener` model keep their existing
names for database compatibility. Public routes, navigation, templates, and
handler names use “proxy”.

## DNS replaces Domains

`/dns` is the only DNS management area. It owns:

- the embedded DNS runtime;
- local zones and records;
- connections to hosted DNS providers and their remote zones;
- upstream resolvers and cache settings;
- filter rules and subscriptions.

The old `/domains` routes remain removed and permanently redirect to `/dns`.
Their provider-backed capability now lives at `/dns/providers`. Local zones and
remote hosted zones remain separate storage models, but both can be selected as
DNS-01 challenge targets. Existing `dns_providers` rows are migrated into
`dns_provider_connections` without exposing credentials in page data.

## Service coordinator

The `/services` page coordinates OhMyServer-owned network runtimes:

- DNS Server;
- Web Server;
- TCP/UDP Proxy.

DHCP and NTP should register through the same interface after their runtimes
are implemented.

Each service exposes:

```go
type managedService interface {
	ID() string
	Name() string
	Description() string
	Status(context.Context, *Server, ServicePreference) (ServiceStatus, error)
	Start(context.Context, *Server) error
	Stop(context.Context, *Server) error
	Reload(context.Context, *Server) error
}
```

`service_preferences` stores the desired global enabled state and last
transition error. Observed state always comes from the actual runtime. This
allows the UI to distinguish a disabled service from a service that should be
running but failed to bind.

## Coordination

Before a service starts or reloads, the coordinator compares its configured
TCP and UDP endpoints with:

- other enabled managed services;
- the OhMyServer control-plane HTTP listener.

Wildcard listeners conflict with every listener on the same protocol and port.
Specific interface addresses conflict only when their addresses match.

Module configuration remains in its existing tables. The coordinator owns only
global desired state and lifecycle operations, so it does not duplicate DNS
zones, Web routes, or proxy listeners.
