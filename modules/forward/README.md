# Forward module

This module provides a TCP/UDP forwarding engine (`proxy`), listener persistence, a module HTTP API, and a standalone management UI.

Run it independently with:

```sh
go run ./cmd/forward web serve --listen 127.0.0.1:8080 --db forward.db
```

Forward listeners bind ports on the local host. Embedding applications may inject their own service adapter to coordinate listener changes with host-wide port-conflict rules and lifecycle policy.
