# zfsmgr

Manage ZFS from the browser, in the order ZFS itself is built:

- **Pools** combine disks into one space. The list shows capacity, state and
  anything needing attention. A pool's page shows its disks as small cards,
  each linked to the disk, starts and cancels scrubs, and lists its datasets.
- **Datasets** are the filesystems in a pool, and can nest. A dataset's page
  shows its space, its schedule and its snapshots, and can create a child.
- **Snapshots** belong to one dataset. They can be taken, compared with the live
  data, browsed, renamed, deleted and rolled back to. The Snapshots page searches
  them across datasets.

This module (id `zfs`, shown as ZFS) is built into OhMyServer for now. It follows
the [module contract](../../docs/module-contract.md) and imports nothing else
from this repository. It drives the `zfs` and `zpool` commands and keeps no state
of its own. Without them the panel leaves it out; after installing ZFS, restart
the panel.

## Creating pools

Creating a pool makes whatever is on its disks unreachable, so it is careful:

- Only disks with nothing on them are offered. One that is mounted, in a pool or
  array, or has a file system or partition table is listed with the reason. It
  has to be cleared by hand first; the page cannot do that. If what the pools are
  made of cannot be read, no disk is offered.
- This never passes `-f`, so zpool refuses a disk it finds in use.
- Disks are named by stable paths (`/dev/disk/by-id/…`), since device names like
  `sdb` can change on reboot.
- Every disk is checked again when creating, a dry run (`zpool create -n`) shows
  the result first, and the pool's name must be typed to confirm.

Removing pools and clearing disks are left to the command line on purpose.

To try this without any real disk, `zfsmgr -test-loop-disks` offers only loop
devices and runs no schedules.

## Creating datasets

A new dataset inherits everything from its parent unless set, including the
schedule. A folder that already holds files where it would mount is refused
before anything is created: zfs would create the dataset and then fail to mount
it. A dataset cannot take over an existing folder; the files have to be copied.

## Schedules

A schedule is stored on the dataset as the user property
`ohmyserver:snapshots`, for example
`hourly=24,daily=14,weekly=4,monthly=6`. Like any ZFS property it is inherited
by child datasets, and it survives reinstalling the panel. No dataset has one
until you set it.

Each pass looks at what exists. When a class's period has started (the hour,
the day, the week from Monday, the month, in the server's time zone) and no
snapshot of the class exists for it, it takes `auto-<class>-YYYYMMDD-HHMM`,
including one for a period missed earlier, then deletes the oldest of that
class beyond its count. Since it goes by what exists, it does not matter when
or how often passes run.

In OhMyServer the job scheduler runs the passes: a job named ZFS snapshots,
created at startup and running each hour, so they show up with their output
and outcome among the other jobs and can be run by hand. The job can be
rescheduled; turned off or deleted, no snapshots are taken or removed and the
ZFS pages say so. A deleted job comes back when a schedule is saved or the panel
restarts. Saving a schedule also starts a pass. On its own, `zfsmgr` runs a pass
each minute instead, and a host program chooses by setting `Options.Trigger` and
calling `Service.Apply` from its job.

Retention is deliberately narrow:

- Only names of exactly that form are ever removed. Renaming one keeps it.
- Setting a class to zero, or removing a schedule, stops taking snapshots and
  removes nothing.
- Held and cloned snapshots stay, and still count toward the number kept.
- If a snapshot cannot be taken, none of that class is removed in that pass.
- An unreadable property value takes and removes nothing, and the page says so.

## Safety

- `zfs destroy` is called in one place, on a name checked to be exactly one
  `dataset@snapshot`. Names with `%` or `,`, which zfs reads as ranges and
  lists, are refused, as are names starting with `-`. It never passes `-r` or
  `-R`.
- Rollback passes `-r` only when the request lists exactly the newer snapshots
  that would be destroyed, as shown to the user; otherwise it answers 409 and
  does nothing. It never passes `-R`, so clones are never destroyed: zfs
  refuses instead.
- Changes continue if the browser goes away, so a bulk delete is not left half
  done without anyone being told.

## Run

```sh
make deps     # at the repository root: the UI's browser dependencies
go build -o zfsmgr ./modules/zfsmgr/cmd/zfsmgr
sudo ZFSMGR_PASSWORD=secret ./zfsmgr -listen 127.0.0.1:8080
```

Without `ZFSMGR_PASSWORD` it serves without authentication, which is only safe
on a loopback address.

## API

Responses are `{"data": …}`, errors `{"error": {"code", "message"}}`. What zfs
itself refuses is 422 `zfs_error` with its message. Names contain slashes, so
they travel in the query string or the body.

| Method | Path | |
|---|---|---|
| GET | `/info` | Whether ZFS is installed, and its version |
| GET | `/pools` | `{pools, details_error}`: capacity and state, with devices and the latest scrub from `zpool status` (OpenZFS 2.3 and later). Asking is limited to five seconds, since zpool can hang on a suspended pool |
| GET | `/datasets` | Filesystems and volumes with snapshot counts and schedules |
| GET | `/dataset?name=` | One of them |
| POST | `/datasets` | `{parent, name, mountpoint, compression, quota}` answers `{name}` |
| GET | `/disks` | The whole disks, with `in_use` saying why one is not offered |
| POST | `/pools/preview` | `{name, layout, disks, mountpoint}` answers `{output, usable, disks, warnings}` |
| POST | `/pools` | The same, creating it |
| POST | `/pools/scrub` | `{pool, action}`, `start` or `stop` |
| PUT | `/policy` | `{dataset, policy: {hourly, daily, weekly, monthly}}`; a null policy removes the dataset's own |
| GET | `/schedule` | Who runs the passes: `{external, job_id, enabled, next_run, last_run, run_status}` |
| GET | `/snapshots?dataset=` | Newest first; without a dataset, all of them |
| GET | `/snapshot?name=` | One, with the newer ones a rollback destroys and its files' path |
| GET | `/snapshot/diff?name=` | `{changes, truncated}`, the first 5000 changes since it |
| POST | `/snapshots` | Take one: `{dataset, name, recursive}` |
| POST | `/snapshots/destroy` | `{names}` answers `{destroyed, failed: [{name, error}]}` |
| POST | `/snapshots/rename` | `{name, new_name}` |
| POST | `/snapshots/rollback` | `{name, destroy_newer}`; 409 `conflict` if they are not exactly the newer ones |
