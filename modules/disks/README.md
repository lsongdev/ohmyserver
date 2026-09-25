# Disks module

The Disks module exposes local block-device and filesystem information through a Go service and a small web UI. It has no module-owned database; the standalone process reads the machine on which it runs.

Run the local UI with:

```sh
go run ./cmd/disks web serve --listen 127.0.0.1:8080
```

The standalone process serves the UI at `/` and the module API, manifest, and assets under `/v1/disks`. Embedding hosts can construct the local service from the public `disks` package and mount `disks/web` under their chosen API prefix.
