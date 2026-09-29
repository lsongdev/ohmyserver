# DNS module

The DNS module owns provider integrations and their context-aware interface, the DNS protocol runtime, HTTP handlers, SQLite schema/repository, subscription refreshes, and its browser assets. The provider contract lives at `providers/types`; ACME DNS-01 support uses ACME’s own narrow challenge capability, which the host adapts.

Run it independently:

```sh
GOWORK=off go run ./cmd/dns web serve --listen 127.0.0.1:8080 --db ./dns.db --list-dir ./dns-lists
```

The standalone process mounts the complete module handler at `/`; an embedding host may mount the same handler at any prefix. DNS listeners are configured through the module settings UI/API. In the panel host, the Server supplies the database repository, coordinates cross-service listener conflicts and desired state, and owns process shutdown.
