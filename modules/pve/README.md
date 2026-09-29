# PVE module

The PVE module owns its Proxmox VE client, saved connection profiles, API, and browser UI. Persistence and static-file serving are implemented in `web/` so embedding and standalone processes use the same handler.

Run it independently with:

```sh
go run ./cmd/pve web serve --listen 127.0.0.1:8080 --db ./pve.db
```

The standalone process mounts the complete module handler at `/`. Embedding hosts initialize the module repository on their shared SQLite connection and mount that same handler at any prefix; all module routes remain relative to it.
