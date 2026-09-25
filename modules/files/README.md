# Files module

`modules/files` is the reusable filesystem package. Its `Service` works within a configured root and can be embedded by other Go applications.

`modules/files/web` provides the HTTP API, module manifest, and embedded browser assets. The host mounts its handler under `/v1/files`; browser code uses the injected API client and does not depend on the host's mount path.

Run the module on its own with:

```sh
go run ./cmd/files web serve --root ./data --listen 127.0.0.1:8080
```

The standalone page is served at `/`. The API and assets are mounted below `/v1/files`. Choose a root directory deliberately: the module only exposes files beneath that root.
