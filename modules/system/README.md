# System module

`system` defines system information DTOs and the host service capability. `system/web` provides the HTTP API, embedded assets and standalone UI.

Run the module UI/API independently with:

```sh
go run ./cmd/system web serve --listen 127.0.0.1:8080
```

The standalone process mounts the complete module handler at `/`; an embedding host may mount the same handler at any prefix. UI, relative API, manifest, and assets always share that module root.
