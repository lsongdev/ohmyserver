# diskmgr

See a Linux server's storage from the browser: how full each mounted file
system is, the state of software RAID (md) arrays including rebuild
progress, and each disk's health from SMART: the drive's own assessment,
temperature, power-on time, solid-state wear, and warnings in plain words
(reallocated or pending sectors, uncorrectable errors, cable CRC errors,
NVMe critical warnings). Self-tests can be started and aborted.

This module (id `disks`, shown as Disks) is built into OhMyServer for now,
and will become a project of its own once it matures. It follows the
[module contract](../docs/module-contract.md) and imports nothing else from
this repository.

It reads `lsblk` (util-linux 2.32 and later), `df` and `/proc/mdstat`, and
SMART data through [smartctl-go](https://github.com/lsongdev/smartctl-go),
which needs smartmontools installed. Disks in standby are never woken by the
pages: their last known health is shown until the operator asks to read
them. Health is cached for five minutes.

## Run

```sh
go install github.com/lsongdev/ohmyserver/diskmgr/cmd/diskmgr@latest
sudo DISKMGR_PASSWORD=secret diskmgr -listen 127.0.0.1:8080
```

## API

| Method | Path | |
|---|---|---|
| GET | `/api/system` | Whether lsblk and smartctl are available |
| GET | `/api/overview` | File systems, RAID arrays, and disks with their health |
| GET | `/api/disks/{name}` | A disk with its full SMART report; `?wake=1` reads a disk in standby |
| POST | `/api/disks/{name}/test` | `{"kind": "short" \| "long" \| "abort"}` |
