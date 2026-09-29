# Disks module

The Disks module exposes local block-device and filesystem information through a Go service and a small web UI. It has no module-owned database; the standalone process reads the machine on which it runs.

Run the local UI with:

```sh
go run ./cmd/disks web serve --listen 127.0.0.1:8080
```

The standalone process mounts the complete module handler at `/`. Embedding hosts construct the same handler from the public `disks` service and may mount it at any prefix; the module keeps all API, page, manifest, and asset paths relative to that root.
