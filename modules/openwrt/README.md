# OpenWrt module

The module provides OpenWrt router management as a Go package, an HTTP API and
an embedded browser UI. `web.Module.Handler()` is the complete module root;
the manifest describes its relative UI entry and navigation.

Run the module on its own with:

```sh
go run ./cmd/openwrt web serve --listen 127.0.0.1:8080 --db openwrt.db
```

The standalone server mounts the handler at `/`; an embedding host may mount
the same handler at any prefix without the module knowing that prefix.
