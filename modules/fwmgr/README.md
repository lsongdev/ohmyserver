# fwmgr

Protect the interfaces that face the internet, and share their connection:
which interfaces are the WAN, whether what leaves through them is translated
(NAT), whether ping is answered, and which ports anyone may connect to.

This module (id `firewall`, shown as Firewall) is built into OhMyServer for
now, and will become a project of its own once it matures. It follows the
[module contract](../../docs/module-contract.md) and imports nothing else from
this repository (the command runner is its own copy).

It writes one nftables table, `inet ohmyserver`, and nothing else. It needs
the `nft` binary (package `nftables`); without it the page says so and changes
are refused.

## What the table does

- Only the interfaces named as WAN are restricted. `input` and `forward`
  chains accept by default, so the local network, loopback and Docker are not
  affected by turning this on.
- From a WAN interface, replies to what this machine started are let in, new
  connections are refused except to the ports allowed, and DHCP and the
  ICMPv6 that IPv6 needs always get through. Ping is answered only if asked.
- New connections arriving on a WAN interface and forwarded to what is behind
  this machine are dropped.
- NAT masquerades IPv4 leaving through a WAN interface. IPv6 is not
  translated. Turning NAT on turns IPv4 forwarding on and writes
  `/etc/sysctl.d/90-ohmyserver-forward.conf`. Turning the firewall off removes
  that file but leaves the running kernel as it is, since other software may
  rely on forwarding.
- A WAN name may end in `*` (`ppp*`), so a dial-up connection that does not
  exist yet is covered when it appears. `lo` and anything matching it is
  refused.
- Docker's own rules live in other tables and are evaluated as well: a drop in
  any of them wins. Docker's `FORWARD` policy is drop, so it may need its own
  rule to let a LAN reach the WAN. That interaction has not been tested.

## Changing it safely

A wrong firewall can cut the panel off, so a change is **on trial**, as in the
network module: the previous firewall is restored after two minutes unless the
change is kept, which needs a request to reach the panel. The record reaches
disk before the change does, so a panel that stops meanwhile resumes the
countdown at its next start, or reverts at once if the time has passed. A
change that nft refuses is undone at once, and the script is checked with
`nft -c` before anything is applied.

The kernel forgets nftables rules at boot, so the saved configuration
(`<state dir>/config.json`) is applied each time the panel starts. Between
boot and the panel starting, the machine has no firewall from this module.

## Run

```sh
go build -o fwmgr ./modules/fwmgr/cmd/fwmgr
sudo FWMGR_PASSWORD=secret ./fwmgr -listen 127.0.0.1:8080
```

`-state` chooses the state directory (default `/var/lib/fwmgr`).

## API

Responses are `{"data": …}`, errors `{"error": {"code", "message", "fields"}}`.

| Method | Path | |
|---|---|---|
| GET | `/api/info` | Whether it runs here, whether `nft` is installed, whether the kernel forwards IPv4 |
| GET | `/api/config` | The saved configuration |
| POST | `/api/config/preview` | The nft script a configuration would apply, checked by `nft -c` |
| PUT | `/api/config` | Apply a configuration on trial |
| GET | `/api/rules` | The table in effect |
| GET | `/api/interfaces` | Interfaces, flagging the uplink and the one this request came through |
| GET | `/api/trial` | The change on trial and how the last one ended |
| POST | `/api/trial/keep` | Save it |
| POST | `/api/trial/revert` | Restore the previous firewall now |
