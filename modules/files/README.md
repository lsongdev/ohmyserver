# Files module

`modules/files` is the reusable filesystem package. Its `Service` works within a configured root and can be embedded by other Go applications.

`modules/files/web` provides one complete handler for the HTTP API, module manifest, browser runtime, and embedded assets. Browser code uses relative routes and does not depend on the host's mount path.

Run the module on its own with:

```sh
go run ./cmd/files web serve --root ./data --listen 127.0.0.1:8080
```

The standalone command mounts that handler at `/`; an embedding host may mount it at any prefix. Choose a root directory deliberately: the module only exposes files beneath that root.
