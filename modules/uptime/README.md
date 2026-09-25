# Uptime module

The Uptime module owns monitored targets, checks, status history, and the browser UI. The public Go package provides the checker contract; `web/` owns persistence, API routes, and embedded assets.

Run it independently with:

```sh
go run ./cmd/uptime web serve --listen 127.0.0.1:8080 --db ./uptime.db
```

The standalone process serves the UI at `/` and the API, manifest, and assets under `/v1/uptime`. Embedding hosts can inject the checker and repository through the module's service constructor.
