# Uptime module

The Uptime module owns monitored targets, checks, status history, and the browser UI. The public Go package provides the checker contract; `web/` owns persistence, API routes, and embedded assets.

Run it independently with:

```sh
go run ./cmd/uptime web serve --listen 127.0.0.1:8080 --db ./uptime.db
```

The standalone process mounts the complete module handler at `/`. Embedding hosts inject the checker and repository through the module's service constructor and may mount that same handler at any prefix.
