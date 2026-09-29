# Widgets module

The Widgets module owns dashboard widget definitions, layout persistence, API routes, and its browser UI. The module repository manages its SQLite schema; embedding hosts should not read or modify widget tables directly.

Run it independently with:

```sh
go run ./cmd/widgets web serve --listen 127.0.0.1:8080 --db ./widgets.db
```

The standalone process mounts the complete module handler at `/`. Embedding applications initialize the module repository on their shared database and may mount that same handler at any prefix; all module routes remain relative to it.
