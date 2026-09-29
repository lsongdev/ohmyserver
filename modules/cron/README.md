# Cron module

The public `cron` package owns job/execution data, SQLite persistence, scheduling, process execution, and lifecycle. `cron/web` exposes the module API, standalone UI, manifest, and embedded ESM assets.

Run it on its own with:

```sh
go run ./cmd/cron web serve --listen 127.0.0.1:8080
```

The database defaults to the user's configuration directory and can be overridden with `--db`. Jobs execute as the operating-system user running the process; use a dedicated service account and restrict command access accordingly.

Embedding applications initialize and start an `Engine`, then mount the complete `web.New(engine).Handler()` at any module root they choose. The standalone command mounts that same handler at `/`; the engine is closed before its database.
