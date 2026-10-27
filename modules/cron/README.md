# cron-go

cron-go lives in OhMyServer's `modules/cron` while it is small and changes with
the panel; it can move back into a project of its own as it is, since it
imports nothing else from the repository. It still runs on its own:
`go run ./modules/cron/cmd/cron-go` from the repository root.

A job scheduler with a web UI: jobs run shell commands on a schedule,
locally or on any target a host program adds (OhMyServer adds its saved SSH
hosts), and every run is recorded with its output and exit code.

- **Schedules**: 5-field cron expressions with names, ranges, steps and
  lists (`30 9 * * mon-fri`, `*/15 * * * *`), the `@daily`-style macros,
  and `@every 10m`. When both day fields are restricted, either one matching
  fires, as in Vixie cron. Each job may have its own time zone; the editor
  previews the next runs.
- **Runs**: run now, cancel, a timeout per job, live output while running.
  A job still running when its next time comes is skipped for that time.
  Runs in progress when the program stops are recorded as interrupted;
  missed times are not made up. The last 100 runs of each job are kept,
  with the last 256 KiB of their output.
- **Targets**: `runner.Local` runs `/bin/sh -c` as the user running the
  program and kills the whole process group on cancel or timeout. Other
  targets implement `runner.Runner`.

One Go binary; jobs live in SQLite. The UI is Preact + HTM loaded as native
ES modules, with no build step.

```sh
CRON_GO_PASSWORD=… go run ./cmd/cron-go -listen 127.0.0.1:8080
```

Jobs run arbitrary commands: never expose cron-go without authentication.

## As a panel module

cron-go follows the panel module contract (see OhMyServer's
`docs/module-contract.md`):

```go
c, err := web.New(ctx, db, web.Options{Runners: []runner.Runner{sshRunner}}) // tables are prefixed cron_
c.Handler() // JSON API, relative routes
c.Assets()  // UI with manifest.json
```

## API

Responses are `{"data": …}`; errors are `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET/POST | `/jobs` | jobs with `next_run`, `running` and `last_run` |
| GET/PUT/DELETE | `/jobs/{id}` | |
| POST | `/jobs/{id}/enable`, `/disable`, `/run` | `run` returns the new run (409 if running) |
| GET | `/jobs/{id}/runs` | newest first, without output |
| GET | `/runs/{id}` | with output; live while running |
| POST | `/runs/{id}/cancel` | |
| GET | `/targets` | where jobs can run |
| GET | `/preview?schedule=&timezone=` | the next 5 times |
| GET | `/summary` | counts for an overview |
