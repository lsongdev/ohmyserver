# Speedtest module

The Speedtest module owns its speed-test client, saved targets and results, HTTP API, and browser UI. Repository and asset handling are kept in `web/` and reused by the standalone command.

Run it independently with:

```sh
go run ./cmd/speedtest web serve --listen 127.0.0.1:8080 --db ./speedtest.db
```

The standalone process serves the UI at `/` and the API, manifest, and assets under `/v1/speedtest`. Embedding hosts provide the module repository and register its handler through the Go composition root.
