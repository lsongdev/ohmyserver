# Settings module

The Settings module owns application key/value settings, validation, SQLite persistence, and the management UI. Its API handler and assets are in `web/`; other modules should consume typed capabilities rather than query its tables.

Run it independently with:

```sh
go run ./cmd/settings web serve --listen 127.0.0.1:8080 --db ./settings.db
```

The standalone process serves the UI at `/` and the API, manifest, and assets under `/v1/settings`. Embedding hosts may share a SQLite connection but leave the settings schema and repository to this module.
