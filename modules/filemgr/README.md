# filemgr

Manage a server's files from the browser: browse folders, view and edit
text files (configuration and the like), preview images, upload (drag and
drop, with progress), download (folders as `.tar.gz`), create folders and
files, rename, move, copy, and delete to a trash.

It is a tool for administering a server, not a media library: it shows the
file system as it is, with permissions and owners, and runs as the user the
panel runs as (usually root).

This module (id `files`, shown as Files) is built into OhMyServer for now,
and will become a project of its own once it matures. It follows the
[module contract](../../docs/module-contract.md) and imports nothing else
from this repository.

## Safety

Deleting is the operation that can do the most damage, so it is never
immediate and never blind (see
[the incident that prompted this](../../docs/incidents/)):

- **Never deleted, moved or renamed:** the root, the system's top-level
  directories (`/etc`, `/usr`, `/var`, …), mount points, home directories
  (from `/etc/passwd`, and `/root`), the trash itself, and any directory
  containing one of them. Their contents can be managed.
- **Preview first:** the server counts the files, folders and bytes a
  deletion affects, refusing to walk into another mounted file system. The
  deletion goes ahead only if that still matches what the preview counted.
  Deleting more than 100 files or 1 GB asks for the name to be typed.
- **Trash, not removal:** deleted items move to `.ohmyserver-trash` at the
  top of their own file system (a rename: nothing is copied) and can be
  restored from the Trash page. Only deleting from the trash removes data,
  again after a preview and a typed confirmation.
- **Atomic writes:** saves and uploads write a temporary file beside the
  target and rename it into place, keeping the file's permissions and owner.
- **Confined paths:** paths never leave the root. A root other than `/`
  limits paths, not symbolic links inside it.
- **Sandboxed previews:** files opened inline are served with a sandboxing
  Content-Security-Policy, so an uploaded HTML or SVG file cannot run as a
  page of the panel.

## Run

```sh
make deps     # at the repository root: the UI's browser dependencies
go build -o filemgr ./modules/filemgr/cmd/filemgr
FILEMGR_PASSWORD=secret ./filemgr -listen 127.0.0.1:8080 -root /srv
```

## API

| Method | Path | |
|---|---|---|
| GET | `/api/list?path=` | A directory and its entries |
| GET | `/api/text?path=` | A text file's content (up to 2 MB) |
| PUT | `/api/text` | `{"path", "text", "create"}` saves a text file |
| GET | `/api/download?path=` | A file (ranges supported; `inline=1` for previews), or a directory as `.tar.gz` |
| POST | `/api/upload?dir=` | Multipart files; `overwrite=1` replaces existing ones |
| POST | `/api/mkdir` | `{"path"}` |
| POST | `/api/rename` | `{"path", "name"}` |
| POST | `/api/move`, `/api/copy` | `{"paths": […], "to": "/dir"}` |
| POST | `/api/delete/preview` | `{"paths": […]}`: what deleting them affects, or why it is refused |
| POST | `/api/delete` | `{"paths": […], "expect": <the preview>}` moves them to the trash |
| GET | `/api/trash` | Trashed items with where they came from |
| POST | `/api/trash/restore` | `{"paths": [<item ids>]}` |
| POST | `/api/trash/preview`, `/api/trash/purge` | Delete trashed items for good, after a preview, the same way |
