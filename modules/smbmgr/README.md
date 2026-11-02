# smbmgr

Manage a Samba file server from the browser: add, change and remove
shares, edit the server settings ([global]) or smb.conf as text, give
system users a Samba password, enable, disable or remove them, and see who
is connected and which files they hold open.

The Logs page reads `smbd.log` from Samba's log directory. It can search
recent entries by IP, user, or error text and refresh every five seconds.
Reads are limited to the latest 1 MiB. It does not change the logging level
or create logs.

This module (id `samba`, shown as Samba) is built into OhMyServer for now,
and will become a project of its own once it matures. It follows the
[module contract](../../docs/module-contract.md) and imports nothing else
from this repository.

It drives Samba's own tools (`testparm`, `smbcontrol`, `smbpasswd`,
`pdbedit`, `smbstatus --json`, Samba 4.16 and later) and keeps no state of
its own: smb.conf and Samba's databases are the source of truth. Without
`smbd` and `testparm` the panel leaves the module out; after installing
Samba, restart the panel.

## Editing smb.conf

- Server settings offers Form and smb.conf modes in the same page. Mode
  switches carry unsaved changes across, including share edits in the full
  file; switching modes never writes or reloads the server configuration.
- Shares and the server settings are edited as parameters. Only the lines
  of the section that changes are rewritten: comments, the order of
  sections and parameters, and parameters the forms have no field for stay
  as they are.
- Every change is checked with `testparm` first; a configuration it rejects
  (or with an unknown parameter, which Samba would ignore) is not saved.
- The previous file is kept as `smb.conf.bak`, the new one replaces it
  atomically with the same permissions, and `smbcontrol smbd
  reload-config` makes smbd read it.
- Each edit carries the version (a hash) of the file it started from; if
  smb.conf changed meanwhile, the edit is refused instead of overwriting.
- Removing a share removes its section only; the folder and its files stay.

## Run

```sh
make deps     # at the repository root: the UI's browser dependencies
go build -o smbmgr ./modules/smbmgr/cmd/smbmgr
sudo SMBMGR_PASSWORD=secret ./smbmgr -listen 127.0.0.1:8080
```

Without `SMBMGR_PASSWORD` it serves without authentication, which is only
safe on a loopback address.

## API

Responses are `{"data": …}`, errors `{"error": {"code", "message"}}`; a
configuration testparm rejects is 422 `invalid_config`, a stale version 409
`conflict`. Edits of smb.conf answer `{version, warnings, reloaded,
reload_error}`.

| Method | Path | |
|---|---|---|
| GET | `/api/info` | Whether Samba is installed, its version, smb.conf's path, smbd's systemd unit |
| GET | `/api/config` | smb.conf as text, with its version |
| POST | `/api/config/preview` | Convert an unsaved draft: `{text, params?}` returns `{text, params}`; params edits only [global], retaining shares and comments |
| POST | `/api/config/check` | testparm's verdict on `{text}`: `{ok, errors, warnings}` |
| PUT | `/api/config` | Replace smb.conf: `{text, version}` |
| POST | `/api/reload` | Ask smbd to reload smb.conf |
| GET, PUT | `/api/global` | [global]'s parameters: `{version, params: [{key, value}]}` |
| GET | `/api/shares` | The shares, summarized with Samba's defaults |
| POST | `/api/shares` | Add a share: `{version, name, params}` |
| GET, PUT | `/api/shares/{name}` | A share's parameters; PUT may rename it with `name` |
| DELETE | `/api/shares/{name}?version=` | Remove a share |
| GET | `/api/users` | Samba users and their account flags |
| GET | `/api/system-users` | System users (uid 1000 and up) that could get a Samba password |
| POST | `/api/users` | Give a system user a Samba password: `{name, password}` |
| PUT | `/api/users/{name}/password` | `{password}` |
| POST | `/api/users/{name}/{action}` | `enable` or `disable` |
| DELETE | `/api/users/{name}` | Remove a user from Samba; the system account stays |
| GET | `/api/status` | Sessions, shares in use and open files (smbstatus) |
| GET | `/api/logs` | Recent smbd.log entries: `q` (case-insensitive text), `limit` (1–5000, default 500) |
