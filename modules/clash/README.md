# Clash module

The Clash module contains three entry points with separate responsibilities:

- `clash/` is the reusable Go client for a Clash-compatible controller API.
- `web/` owns the module API, SQLite repository, embedded browser UI and the
  handler used by OhMyServer's composition root.
- `cmd/clash` starts the same Web service as a standalone application.

## Run standalone

```sh
go run ./cmd/clash web serve
```

The service listens on `127.0.0.1:8080`, serves its management UI at `/`, and
stores its database at the platform's user configuration directory under
`clash/clash.db`. Use `--listen` and `--db` to override those defaults. The
module API, discovery manifest, and assets share that same root.

## Embed in another Go application

```go
repository := web.NewRepository(db)
if err := repository.Initialize(ctx); err != nil {
    return err
}
handler := web.New(web.NewManager(repository)).Handler()
```

The host mounts this complete handler at any chosen module root. The browser
module calls only relative API and page routes; it never reads or constructs
the host prefix. The standalone command mounts the same handler at `/`.

## Use the Go client

```go
client := clash.NewClient("http://127.0.0.1:9090", "controller-secret")
config, err := client.GetConfig(ctx)
```

Controller credentials are kept in the module repository and are omitted from
API DTOs. This module currently uses SQLite for its configuration repository.
