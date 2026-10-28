# diskmgr

See a server's storage (Linux or macOS) from the browser: how full each mounted file
system is, the state of software RAID (md) arrays including rebuild
progress, and each disk's health from SMART: the drive's own assessment,
temperature, power-on time, solid-state wear, and warnings in plain words
(reallocated or pending sectors, uncorrectable errors, cable CRC errors,
NVMe critical warnings). Self-tests can be started and aborted.

This module (id `disks`, shown as Disks) is built into OhMyServer for now,
and will become a project of its own once it matures. It follows the
[module contract](../../docs/module-contract.md) and imports nothing else from
this repository.

On Linux it reads `lsblk` (util-linux 2.32 and later), `df` and
`/proc/mdstat`. On macOS it reads `diskutil` (physical disks, their
partitions, and the APFS containers and volumes on them), `df` and `mount`;
volumes hidden from the Finder are left out, and an APFS container shows
once, with the space all its volumes use. RAID arrays are Linux only. Both
read SMART data through [smartctl-go](https://github.com/lsongdev/smartctl-go),
which needs smartmontools installed. Disks in standby are never woken by the
pages: their last known health is shown until the operator asks to read
them. Health is cached for five minutes.

## Run

```sh
make deps     # at the repository root: the UI's browser dependencies
go build -o diskmgr ./modules/diskmgr/cmd/diskmgr
sudo DISKMGR_PASSWORD=secret ./diskmgr -listen 127.0.0.1:8080
```

## API

| Method | Path | |
|---|---|---|
| GET | `/api/system` | Whether the disk tools (lsblk or diskutil) and smartctl are available, and the OS |
| GET | `/api/overview` | File systems, RAID arrays, and disks with their health |
| GET | `/api/disks/{name}` | A disk with its full SMART report; `?wake=1` reads a disk in standby |
| POST | `/api/disks/{name}/test` | `{"kind": "short" \| "long" \| "abort"}` |
