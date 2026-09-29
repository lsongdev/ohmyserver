# Speedtest module

The Speedtest module owns its speed-test client, saved targets and results, HTTP API, and browser UI. Repository and asset handling are kept in `web/` and reused by the standalone command.

Run it independently with:

```sh
go run ./cmd/speedtest web serve --listen 127.0.0.1:8080 --db ./speedtest.db
```

The standalone process mounts the complete module handler at `/`. Embedding hosts provide the module repository and mount that same handler at any prefix through the Go composition root.
