# Monitor module

The Monitor module collects local system statistics, persists history, and serves the monitor API and browser UI. Its public Go package owns metric types and collection; `web/` owns the SQLite repository and HTTP/static-file layer.

Run the module independently with:

```sh
go run ./cmd/monitor web serve --listen 127.0.0.1:8080 --db ./monitor.db
```

The standalone process serves the UI at `/` and the API, manifest, and assets under `/v1/monitor`. Embedding applications may provide a collector and repository using the module's narrow interfaces.
