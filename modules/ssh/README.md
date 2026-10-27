# ssh-go

ssh-go lives in OhMyServer's `modules/ssh` while it is small and changes with
the panel; it can move back into a project of its own as it is, since it
imports nothing else from the repository. It still runs on its own:
`go run ./modules/ssh/cmd/ssh-go` from the repository root.

Saved SSH hosts and a terminal on each, in the browser.

- Hosts authenticate with a password, a private key (optionally encrypted),
  or the **panel key**: an ed25519 key ssh-go generates once; add its public
  key to `~/.ssh/authorized_keys` and no secret needs to be stored.
- Host keys are pinned on first connection. A server that later offers a
  different key is refused until you clear the pinned key in the UI.
- The terminal is xterm.js over a WebSocket, with the same protocol as
  docker-go's container terminal: binary frames carry terminal bytes, text
  frames carry `{"type":"resize","cols":…,"rows":…}`, and the socket closes
  with the shell's exit status. Dead connections are detected with SSH
  keepalives.

Passwords and private keys are stored in the SQLite database as given:
protect the database file, and never expose ssh-go without authentication —
it is a gateway to every saved host.

```sh
SSH_GO_PASSWORD=… go run ./cmd/ssh-go -listen 127.0.0.1:8080
```

## As a panel module

ssh-go follows the panel module contract (see OhMyServer's
`docs/module-contract.md`):

```go
s, err := web.New(ctx, db, web.Options{}) // tables are prefixed ssh_
s.Handler()                               // JSON API + WebSocket terminal, relative routes
s.Assets()                                // UI with manifest.json
```

## API

Responses are `{"data": …}`; errors are `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET/POST | `/hosts` | secrets are never returned, only `has_password` / `has_key` |
| GET/PUT/DELETE | `/hosts/{id}` | on update, empty secrets keep the stored ones |
| POST | `/test` | connect without saving; `id` merges a stored host |
| DELETE | `/hosts/{id}/host-key` | forget the pinned host key |
| GET | `/hosts/{id}/terminal?cols=&rows=` | WebSocket; close code 4000 connect failed, 4001 host key changed |
| GET | `/key` | the panel's public key |
| GET | `/summary` | counts for an overview |
