# ci-go

ci-go lives in OhMyServer's `modules/ci` while it is small and changes with
the panel; it can move back into a project of its own as it is, since it
imports nothing else from the repository. It still runs on its own:
`make deps`, then `go run ./modules/ci/cmd/ci-go` from the repository
root.

A small CI for one Docker host: a push (webhook) or a click builds a
repository by running the steps in its `.ci.yml`, each in a container of its
own, with the log live in the browser and the history kept.

```yaml
# .ci.yml — a subset of Woodpecker's format
when:
  branch: [main, release/*]   # glob patterns
  event: [push, manual]
steps:                        # a list, or a map of name to step (in order)
  test:
    image: golang:1.26
    commands:
      - go vet ./...
      - go test ./...
    environment: {CGO_ENABLED: "0"}
    secrets: [API_TOKEN]      # repository secrets, masked in logs
    cache: [/root/go/pkg/mod, /root/.cache/go-build]
  image:
    build:                    # run by ci-go, not by the step
      tag: myapp:latest
      args: {VERSION: "1.0"}
```

- **Builds**: the repository is cloned into a fresh volume (`alpine/git`);
  steps run in order with it at `/workspace`, each command echoed and the
  first failure stopping the build. Builds can be cancelled, restarted and
  time out; one runs at a time by default, the others queue.
- **Caches**: `cache` paths are named volumes per repository and path, kept
  between builds.
- **Images**: `build` steps run `docker build` in a `docker:cli` container
  ci-go starts with the daemon's socket. Steps themselves never get the
  daemon.
- **Webhooks**: `POST /api/hooks/{repo}` takes push events from GitHub,
  Gitea (HMAC-SHA256 signatures) and GitLab (the secret as a token); tags and
  deleted branches are ignored.
- **Clean up**: workspaces and containers are removed after each build, and
  whatever a crashed process left (labelled `ci-go.build`) at startup.

Steps run arbitrary code from the repository: add only repositories you
trust.

```sh
CI_GO_PASSWORD=… go run ./cmd/ci-go -listen 127.0.0.1:8080
```

The password guards everything but `/api/hooks/`, which webhooks
authenticate with their signatures. Docker is found the way the docker CLI
finds it (`DOCKER_HOST`, the current context, `/var/run/docker.sock`).

## As a panel module

ci-go follows the panel module contract (see OhMyServer's
`docs/module-contract.md`); its manifest lists `/hooks/` as public, so a
panel serves webhooks without its sign-in:

```go
c, err := web.New(ctx, db, web.Options{}) // tables are prefixed ci_
c.Handler() // JSON API, relative routes
c.Assets()  // UI with manifest.json
```

## API

Responses are `{"data": …}`; errors are `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET/POST | `/repos` | secrets only by name; the token is never returned |
| GET/PUT/DELETE | `/repos/{id}` | `GET` includes the webhook secret |
| POST | `/repos/{id}/webhook-secret` | replace the webhook secret |
| GET/POST | `/repos/{id}/builds` | `POST {"branch"}` queues a manual build |
| GET | `/builds`, `/builds/{id}` | a build with its steps |
| GET | `/builds/{id}/steps/{n}/log?offset=` | `{text, offset, done}`; follow by passing `offset` back |
| POST | `/builds/{id}/cancel`, `/builds/{id}/restart` | |
| POST | `/hooks/{id}` | webhooks (public) |
| GET | `/summary` | counts for an overview |
