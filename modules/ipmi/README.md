# IPMI module

The IPMI module provides a reusable client for IPMI/BMC operations, a repository for saved server entries, and the matching HTTP API and browser UI. Database access and static assets are owned by `web/`.

Run it independently with:

```sh
go run ./cmd/ipmi web serve --listen 127.0.0.1:8080 --db ./ipmi.db
```

The standalone process serves the UI at `/` and the API, manifest, and assets under `/v1/ipmi`. Embedding hosts provide the shared SQLite connection or repository and mount the module handler at their registered API prefix.
