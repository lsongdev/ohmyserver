# Dashboard module

The Dashboard module owns the overview API contract and UI. Hosts can inject
widget and system summary sources through `LocalService`; the standalone
command supplies local machine metrics and runs without a database.

```sh
go run ./cmd/dashboard web serve --listen 127.0.0.1:8080
```

The standalone command reports local CPU, memory, disk, uptime, and load. It
does not include shell-managed widgets because those are supplied by the host's
Widgets adapter.
