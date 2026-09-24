# Terminal module

The web package owns WebSocket transport and browser assets. Embedding hosts can
provide local, SSH, and container sessions through the `Service` interface.

Run the module independently for a local shell with:

```sh
go run ./cmd/terminal web serve --listen 127.0.0.1:8080
```

The standalone command provides the local shell endpoint and embedded UI. SSH
and container sessions require an embedding host to provide those adapters.
