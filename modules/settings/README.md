# Settings module

The Settings module owns application key/value settings, validation, SQLite persistence, and the management UI. Its API handler and assets are in `web/`; other modules should consume typed capabilities rather than query its tables.

Run it independently with:

```sh
go run ./cmd/settings web serve --listen 127.0.0.1:8080 --db ./settings.db
```

The standalone process mounts the complete module handler at `/`; embedding hosts may mount the same handler at any prefix. Hosts may share a SQLite connection but leave the settings schema and repository to this module.
