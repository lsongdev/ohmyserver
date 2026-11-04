# netmgr

See and change this machine's network from the browser: interfaces with their
addresses and live traffic, the routing table, DNS servers, listening ports,
and diagnostics (ping, trace route, DNS lookup, TCP port check) run from the
server's side.

This module (id `network`, shown as Network) is built into OhMyServer for now,
and will become a project of its own once it matures. It follows the
[module contract](../../docs/module-contract.md) and imports nothing else
from this repository.

It reads through iproute2 (`ip`, `ss`, optionally `resolvectl`) and changes
settings through whichever network manager runs here: NetworkManager (`nmcli`,
connection profiles) or systemd-networkd (`.network` files). With neither it
only reads. Outside Linux the panel leaves the module out.

## Changing settings safely

A wrong address or gateway can cut a server off from the very page that could
fix it, so every change is **on trial**:

1. The record of how to undo it reaches disk (`<state dir>/trial.json`), then
   the change is made. If applying fails, the previous settings return at once.
2. It reverts by itself after two minutes unless someone keeps it. Keeping it
   needs a request to reach the panel, which proves a connection still works.
3. If the panel stops meanwhile, its next start waits out the time left, or
   reverts at once if the time has passed. If reverting fails, it is retried
   until it succeeds.

Only one change can be on trial at a time, and the banner on every page shows
the countdown. The commands run independently of the request, which the change
itself may end.

Interfaces other software looks after (loopback, Docker bridges, veth, tun,
WireGuard, ports of a bridge or bond) cannot be changed here. An interface that
carries the default route, or this client's connection, cannot be turned off.

### systemd-networkd

- Only the keys the form owns are rewritten: `DHCP`, `Address`, `Gateway`,
  `DNS`, `MTUBytes` and `[Route]` sections with just `Destination` and
  `Gateway`. Comments, order and all else stay, and the page lists what it
  keeps. The previous file is kept as `<file>.bak`.
- An interface no file matches gets `10-ohmyserver-<name>.network`. A file
  under `/usr/lib` is copied to `/etc/systemd/network` under the same name,
  which replaces it.
- These are refused rather than risk changing the wrong thing: files generated
  under `/run` (netplan), interfaces with drop-ins, a `[Match]` that may cover
  other interfaces, and a file another interface also uses.
- Each edit carries the version (a hash) of what it started from; if the file
  changed meanwhile, the edit is refused.

### NetworkManager

- The connection profile the device uses is changed with `nmcli connection
  modify`, then `nmcli device reapply`, falling back to `nmcli connection up`.
- An ethernet device without a profile gets one named `ohmyserver-<name>`. The
  IPv6 method stays as it was (`disabled`, `ignore`) unless the form says
  otherwise. Route metrics and attributes have no field and are dropped when
  saving; the preview shows this.

## Run

```sh
make deps     # at the repository root: the UI's browser dependencies
go build -o netmgr ./modules/netmgr/cmd/netmgr
sudo NETMGR_PASSWORD=secret ./netmgr -listen 127.0.0.1:8080
```

Without `NETMGR_PASSWORD` it serves without authentication, which is only safe
on a loopback address. `-state` chooses the state directory (default
`/var/lib/netmgr`).

## API

Responses are `{"data": …}`, errors `{"error": {"code", "message", "fields"}}`;
an invalid configuration is 422 `invalid`, with messages per field, a stale
version 409 `conflict`, and a refused change 409 `in_use`.

| Method | Path | |
|---|---|---|
| GET | `/api/info` | Whether it can run here, the network manager, the host name |
| GET | `/api/interfaces` | Interfaces with addresses and counters |
| GET | `/api/interfaces/{name}` | One of them |
| POST | `/api/interfaces/{name}/{up,down}` | Turn it on or off |
| GET | `/api/interfaces/{name}/config` | Its stored settings: `{config, version, target, preserved, reason, protected}` |
| POST | `/api/interfaces/{name}/config/preview` | What `{version, config}` would change: `{target, before, after}` |
| PUT | `/api/interfaces/{name}/config` | Apply `{version, config}` on trial |
| GET | `/api/trial` | The change on trial and how the last one ended |
| POST | `/api/trial/keep` | Make it permanent |
| POST | `/api/trial/revert` | Restore the previous settings now |
| GET | `/api/routes` | The main routing tables |
| GET | `/api/resolvers` | DNS servers, from resolv.conf and systemd-resolved |
| GET | `/api/ports` | Listening sockets and their processes |
| POST | `/api/tools/run` | Run `{tool, target, port?, record?}`, with tool `ping`, `trace`, `lookup` or `port`; streams plain text |
