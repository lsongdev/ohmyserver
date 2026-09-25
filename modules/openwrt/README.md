# OpenWrt module

The module provides OpenWrt router management as a Go package, an HTTP API and
an embedded browser UI. The shell application mounts `web.Module.Handler()` at
`/v1/openwrt`; the module manifest describes the UI entry and navigation.

Run the module on its own with:

```sh
go run ./cmd/openwrt web serve --listen 127.0.0.1:8080 --db openwrt.db
```

The standalone server serves the module UI at `/` and the API and static assets
under `/v1/openwrt`.
