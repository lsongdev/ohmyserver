# System module

`system` defines system information DTOs and the host service capability. `system/web` provides the HTTP API, embedded assets and standalone UI; host-specific update behavior remains an injected `Service` capability.

Run the module UI/API independently with:

```sh
go run ./cmd/system web serve --listen 127.0.0.1:8080
```

The standalone process serves the UI at `/` and API/assets under `/v1/system`. Update installation is intentionally unavailable in the standalone command because it has no host updater.
