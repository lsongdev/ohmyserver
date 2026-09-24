# PVE module

The PVE module owns its Proxmox VE client, saved connection profiles, API, and browser UI. Persistence and static-file serving are implemented in `web/` so embedding and standalone processes use the same handler.

Run it independently with:

```sh
go run ./cmd/pve web serve --listen 127.0.0.1:8080 --db ./pve.db
```

The standalone process serves the UI at `/` and the API, manifest, and assets under `/v1/pve`. Embedding hosts initialize the module repository on their shared SQLite connection and register the handler under the module's API prefix.
