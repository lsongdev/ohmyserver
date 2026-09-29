# Docker module

The module owns the Docker HTTP API, browser assets, module manifest, DTOs, app-catalog persistence, Docker SDK client, container/image/resource operations, Compose commands, build contexts, stats, and app installation tasks. The host can still supply a `Service` adapter to enforce host-wide deployment policy.

Run the module independently with:

```sh
go run ./cmd/docker web serve --listen 127.0.0.1:8080 --db ./docker.db --apps-dir ./apps-catalog --compose-dir ./compose
```

Set Docker's standard `DOCKER_HOST`/TLS environment variables when the daemon is not using its local default. The standalone process mounts the complete module handler at `/`.

Module UI resources are in `web/assets`. Embedding applications provide the shared SQLite repository and host policy adapter, then mount the same handler at any module root; all module routes remain relative to it.
