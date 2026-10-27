# svcmgr

Manage a Linux server's systemd services and read its journal from the
browser: list services with their state, start, stop, restart, reload,
enable and disable them, read their unit files, and search or follow the
logs of one service or the whole system, live.

This module (id `services`, shown as Services) is built into OhMyServer for
now, and will become a project of its own once it matures. It already
follows the [module contract](../../docs/module-contract.md) and imports
nothing else from this repository, so moving it out is a matter of copying
this directory and adding a go.mod.

It drives `systemctl` and `journalctl` (systemd 239 and later) and keeps no
state of its own.

## Safety

- The service running the panel can be restarted (without waiting, since
  the restart ends the process) but not stopped or disabled from here.
- Services whose start shuts down, reboots or switches the system to rescue
  mode (`systemd-reboot`, `rescue`, …) are never started from here.
- Names are validated and passed as arguments, never through a shell.

## Run

```sh
make deps     # at the repository root: the UI's browser dependencies
go build -o svcmgr ./modules/svcmgr/cmd/svcmgr
sudo SVCMGR_PASSWORD=secret ./svcmgr -listen 127.0.0.1:8080
```

Without `SVCMGR_PASSWORD` it serves without authentication, which is only
safe on a loopback address.

## API

Responses are `{"data": …}`, errors `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET | `/api/system` | Whether systemd runs, its version, and the service running the panel |
| GET | `/api/services` | Services: loaded ones, and installed ones that are not loaded |
| GET | `/api/services/{name}` | A service's state: since when, main process, memory, CPU, tasks, restarts |
| GET | `/api/services/{name}/file` | Its unit file with drop-ins, as `systemctl cat` prints it |
| POST | `/api/services/{name}/{action}` | `start`, `stop`, `restart`, `reload`, `enable` or `disable` |
| GET | `/api/logs` | Journal entries: `unit`, `lines` (≤ 5000), `priority` (1–7), `since` (a duration such as `1h`), `q` (text in the message) |
| GET | `/api/logs/stream` | The same filters, followed live as server-sent `entry` events; `after` is the cursor to continue from |
