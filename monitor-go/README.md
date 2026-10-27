# monitor-go

A host monitor with a web UI: CPU, load, memory and swap, disk usage and
I/O, network traffic, and host details, sampled every 15 seconds, kept as
history and drawn as charts.

- **Local**: `collector.Local` reads this machine through gopsutil (Linux,
  macOS, Windows, BSD; no cgo).
- **Remote Linux hosts without an agent**: `procfs.Script` prints what
  `/proc` and a few commands say in one round trip, and `procfs.Parse`
  reads it; it needs only a POSIX shell (busybox is enough). A host program
  runs the script however it reaches the host — OhMyServer runs it over
  SSH for the hosts saved in ssh-go.
- **Totals that mean something**: physical interfaces and whole disks only
  (no loopback, veth, bridges or partitions), file systems backed by a
  device (no tmpfs, overlay, snaps, disk images or macOS system volumes),
  each device once.
- **History that stays small**: raw samples for a day, one-minute averages
  for a week, one-hour averages for three months. Queries read the finest
  resolution available for the period and return about 300 points.
- Charts are plain SVG (no chart library), with gaps where data is missing.

Any other source implements `collector.Collector`.

```sh
go run ./cmd/monitor-go -listen 127.0.0.1:8080   # -db defaults to the user config dir
```

Set `MONITOR_GO_PASSWORD` to require HTTP basic authentication.

## As a panel module

monitor-go follows the panel module contract (see OhMyServer's
`docs/module-contract.md`):

```go
m, err := web.New(ctx, db, web.Options{Collectors: []collector.Collector{sshHosts}}) // tables are prefixed monitor_
m.Handler() // JSON API, relative routes
m.Assets()  // UI with manifest.json
```

## API

Responses are `{"data": …}`; errors are `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET | `/hosts?spark=1` | hosts with status, info, latest sample, file systems; `spark` adds the last hour of CPU and memory |
| GET | `/hosts/{id}` | one host |
| GET | `/hosts/{id}/series?range=1h\|6h\|24h\|7d\|30d\|90d` | `{step, points}` |
| POST | `/hosts/{id}/enable`, `/disable` | history is kept while disabled |
| DELETE | `/hosts/{id}` | history of a host its collector no longer offers |
| GET | `/summary` | counts and the local host, for an overview |
