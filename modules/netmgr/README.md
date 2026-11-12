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

A trial is not tied to interface settings: it holds a *subject* (what the
change is about), a target, and an undo that lists every NetworkManager
connection profile the change touched. Reverting walks that list backwards,
deleting profiles the change created and restoring the ones it modified. Other
kinds of change share the same safety net.

Keeping a change proves the panel is reachable from where the client is, which
says nothing about a new uplink. The banner therefore has a **Check
connection** button that tests, in order, the default route, the gateway, DNS
and a TCP connection to a public address. It informs the decision and never
blocks it.

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
- The route priority (`metric`) sets `ipv4.route-metric` and `ipv6.route-metric`
  together. Empty is `-1`, which leaves it to NetworkManager (about 100 for
  Ethernet and 600 for Wi-Fi). When several interfaces have a default route,
  the lowest number carries the traffic. systemd-networkd keeps the priority
  per route and per DHCP lease, which the form cannot express, so there a
  non-zero value is refused (`no_metric` in the settings response).
- An ethernet device without a profile gets one named `ohmyserver-<name>`. The
  IPv6 method stays as it was (`disabled`, `ignore`) unless the form says
  otherwise. Route metrics and attributes have no field and are dropped when
  saving; the preview shows this.

### Bridges

A bridge joins Ethernet interfaces into one network. It is made through
NetworkManager only, and every change to it is on trial like any other.

- A bridge is a connection profile (`ohmyserver-<name>`), and each port gets a
  profile that names it as its master. A port without a profile gets
  `ohmyserver-<port>`. A port gives up its own address; the bridge's addresses
  are set afterwards in its interface settings, like any interface's. A new
  bridge starts with none.
- Only Ethernet interfaces can be ports, and not ones another bridge or other
  software has taken. Names Docker generates (`br-` and twelve hex digits),
  `docker0` and `virbr*` cannot be used for a bridge; `br-lan` can.
- Removing a bridge frees its ports, which ask for an address again, and
  brings them up on their own devices. Reverting it makes the bridge again and
  puts the ports back with the settings they had.
- Ports that stay in a bridge when it changes are not touched, so they keep
  their link.
- A bridge or port whose change fails part-way is restored at once. Reverts
  can be retried: a profile already gone is not an error.

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
| GET | `/api/info` | Whether it can run here, the network manager, the host name, and with NetworkManager its `features`: Wi-Fi devices and whether PPPoE is ready (`pppoe.missing` names what to install) |
| GET | `/api/interfaces` | Interfaces with addresses and counters |
| GET | `/api/interfaces/{name}` | One of them |
| POST | `/api/interfaces/{name}/{up,down}` | Turn it on or off |
| GET | `/api/interfaces/{name}/config` | Its stored settings: `{config, version, target, preserved, reason, protected}` |
| POST | `/api/interfaces/{name}/config/preview` | What `{version, config}` would change: `{target, before, after}` |
| PUT | `/api/interfaces/{name}/config` | Apply `{version, config}` on trial |
| GET | `/api/trial` | The change on trial and how the last one ended: `{trial: {subject, target, deadline, seconds}, last: {subject, result, at}}` |
| POST | `/api/connectivity/check` | Test the route, gateway, DNS and internet: `{checks: [{id, ok, detail}]}` |
| POST | `/api/trial/keep` | Make it permanent |
| POST | `/api/trial/revert` | Restore the previous settings now |
| GET | `/api/bridges` | Bridges with their ports and spanning tree setting |
| GET | `/api/bridges/{name}` | One of them, with the `version` to send back |
| PUT | `/api/bridges/{name}` | Make or change `{version, config: {stp, ports}}` on trial; `version` is empty for a new bridge |
| DELETE | `/api/bridges/{name}?version=…` | Remove it on trial |
| GET | `/api/routes` | The main routing tables |
| GET | `/api/resolvers` | DNS servers, from resolv.conf and systemd-resolved |
| GET | `/api/ports` | Listening sockets and their processes |
| POST | `/api/tools/run` | Run `{tool, target, port?, record?}`, with tool `ping`, `trace`, `lookup` or `port`; streams plain text |
