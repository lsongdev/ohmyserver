# zfsmgr

Manage ZFS snapshots from the browser: take them, see how much space they
hold, compare one with the live data, browse its files, rename, delete and
roll back, and schedule automatic ones with retention.

This module (id `zfs`, shown as ZFS Snapshots) is built into OhMyServer for
now. It follows the [module contract](../../docs/module-contract.md) and
imports nothing else from this repository. It drives the `zfs` and `zpool`
commands and keeps no state of its own. Without them the panel leaves it out;
after installing ZFS, restart the panel.

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
| GET | `/pools` | Capacity and health of the pools |
| GET | `/datasets` | Filesystems and volumes with snapshot counts and schedules |
| GET | `/dataset?name=` | One of them |
| PUT | `/policy` | `{dataset, policy: {hourly, daily, weekly, monthly}}`; a null policy removes the dataset's own |
| GET | `/schedule` | Who runs the passes: `{external, job_id, enabled, next_run, last_run, run_status}` |
| GET | `/snapshots?dataset=` | Newest first; without a dataset, all of them |
| GET | `/snapshot?name=` | One, with the newer ones a rollback destroys and its files' path |
| GET | `/snapshot/diff?name=` | `{changes, truncated}`, the first 5000 changes since it |
| POST | `/snapshots` | Take one: `{dataset, name, recursive}` |
| POST | `/snapshots/destroy` | `{names}` answers `{destroyed, failed: [{name, error}]}` |
| POST | `/snapshots/rename` | `{name, new_name}` |
| POST | `/snapshots/rollback` | `{name, destroy_newer}`; 409 `conflict` if they are not exactly the newer ones |
