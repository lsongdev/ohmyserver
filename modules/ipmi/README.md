# IPMI module

The IPMI module provides a reusable client for IPMI/BMC operations, a repository for saved server entries, and the matching HTTP API and browser UI. Database access and static assets are owned by `web/`.

Run it independently with:

```sh
go run ./cmd/ipmi web serve --listen 127.0.0.1:8080 --db ./ipmi.db
```

The standalone process mounts the complete module handler at `/`. Embedding hosts provide the shared SQLite connection or repository and mount that same handler at any prefix; all module routes remain relative to it.
