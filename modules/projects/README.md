# Projects module

Projects owns project and deployment records, Git checkout/build/deploy orchestration,
the HTTP API, and its embedded browser UI. The embedding host supplies a narrow
Docker runtime implementation through `web.DeploymentRuntime`.

Run the complete module independently with Docker and Git available on the host:

```sh
go run ./cmd/projects web serve --listen 127.0.0.1:8080 --db ./projects.db --workspace ./workspace
```

The standalone server mounts the complete module handler at `/`; an embedding host may mount that same handler at any prefix. The `--docker` option selects the Docker CLI executable.
