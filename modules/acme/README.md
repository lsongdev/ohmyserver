# ACME module

The public `acme` package owns ACME DTOs, SQLite schema/repository, CSR and certificate validation, and the issuance engine. `acme/web` provides the HTTP API, standalone UI, module manifest, and embedded assets. The host may inject a DNS-01 capability through `acme.DNSChallenge`; the standalone command currently enables HTTP-01 and leaves DNS-01 unavailable until a DNS adapter is supplied.

Run the module independently with:

```sh
go run ./cmd/acme web serve --listen 127.0.0.1:8080
```

To use HTTP-01, expose `/.well-known/acme-challenge/` on the requested certificate domains to this process (typically by binding the public HTTP listener or routing that path through a reverse proxy). The database defaults to the user's configuration directory and can be overridden with `--db`.

`web.New(engine).Handler()` is the complete module root: it serves the UI, manifest, assets, shared browser runtime, and relative API routes. The standalone command mounts it at `/`; embedding applications choose any mount prefix and close the engine before closing its database.
