# filemgr

Manage a server's files from the browser: browse folders, view and edit
text files (configuration and the like), preview images, upload (drag and
drop, with progress), download (folders as `.tar.gz`), create folders and
files, rename, move, copy and delete.

It is a tool for administering a server, not a media library: it shows the
file system as it is, with permissions and owners, and runs as the user the
panel runs as (usually root).

This module (id `files`, shown as Files) is built into OhMyServer for now,
and will become a project of its own once it matures. It follows the
[module contract](../../docs/module-contract.md) and imports nothing else
from this repository.

## Safety

- The root (`/` by default) and the system's top-level directories (`/etc`,
  `/usr`, `/var`, …) cannot be deleted, moved or renamed; their contents can.
- Saves and uploads write a temporary file beside the target and rename it
  into place, so an interrupted write never leaves a half-written file.
  Saving keeps the file's permissions and owner.
- Paths never leave the root. A root other than `/` limits paths, not
  symbolic links inside it.
- Files opened inline (image previews) are served with a sandboxing
  Content-Security-Policy, so an uploaded HTML or SVG file cannot run as a
  page of the panel.

## Run

```sh
go install github.com/lsongdev/ohmyserver/modules/filemgr/cmd/filemgr@latest
FILEMGR_PASSWORD=secret filemgr -listen 127.0.0.1:8080 -root /srv
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
| POST | `/api/delete` | `{"paths": […]}` |
