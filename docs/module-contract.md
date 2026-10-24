# Module contract

This document is the agreement between the panel **shell** (OhMyServer;
implemented in `internal/shell`) and **modules** (independent applications
such as docker-go, acme-go and yabber). A module that follows it runs on its own and
can be mounted by any shell, without importing any shell code.

Contract version: **1**.

## Principles

- **Modules never depend on the shell.** No Go import, no JavaScript import.
  The contract is expressed in standard-library types, a JSON manifest and
  one JavaScript function.
- **The shell discovers, the module describes.** Everything the shell needs
  to know is in the module's manifest.
- **Minimal dependencies.** A module depends only on what it needs to run
  on its own. Whoever uses something implements it: a module ships a SQLite
  store only if it uses SQLite standalone. A shell that wants a module's
  state somewhere else implements the module's store interface itself.
- **One code path.** Standalone and mounted, a module runs the same
  constructor, handler, store and UI. Only the surrounding host differs.

## 1. Go interface

The shell defines the interface; modules satisfy it implicitly:

```go
// Defined by the shell. Modules do not import it.
type Module interface {
	Handler() http.Handler // the module's API, with relative routes
	Assets() fs.FS         // browser assets; manifest.json at the root
}
```

If the module also implements `io.Closer`, the shell calls `Close` at
shutdown, after the HTTP server has stopped.

By convention, a module's constructor takes only standard-library types and
the module's own option types:

```go
func New(ctx context.Context, opts Options) (*Module, error)
// a module that uses SQLite on its own also takes the database:
func New(ctx context.Context, db *sql.DB, opts Options) (*Module, error)
```

`opts` carries the module's bootstrap values (a data directory, optional
collaborators such as an `apps.Exposer`, optional store implementations). Cross-module collaboration uses
interfaces the **consuming** module defines. The shell's assembly code
implements them with adapters, so modules never import each other.

### Mounting

| | Mounted by a shell | Standalone (recommended) |
|---|---|---|
| API (`Handler`) | `/m/<id>/api/` | `/api/` |
| Assets (`Assets`) | `/m/<id>/ui/` | `/` |

The handler sees paths relative to its mount point (the prefix is stripped).
It must not assume an absolute path, and it must not hard-code the prefix in
links or redirects.

### Authentication

When mounted, the shell authenticates every request before it reaches the
module's handler: modules must be constructable with their own
authentication disabled. Standalone, a module authenticates however it likes
(password, token, reverse proxy).

Write requests reach the handler only from the panel's own origin: the shell
applies `http.CrossOriginProtection`. Modules should apply it themselves when
standalone.

## 2. Manifest

`Assets()` contains `manifest.json` at its root:

```json
{
  "contract": 1,
  "id": "docker",
  "title": "Docker",
  "icon": "icon.svg",
  "entry": "module.js",
  "styles": ["style.css", "docker.css"],
  "home": "/",
  "nav": [
    { "path": "/", "title": "Dashboard", "icon": "◎", "group": "Overview" },
    { "path": "/containers", "title": "Containers", "icon": "▣", "group": "Resources" }
  ]
}
```

| Field | Required | Meaning |
|---|---|---|
| `contract` | yes | Contract version. A shell refuses versions it does not support. |
| `id` | yes | `[a-z][a-z0-9-]*`, unique within a shell; used in URLs and table names. |
| `title` | yes | Navigation label. A shell may override it. |
| `icon` | no | A path inside the assets (SVG or PNG), or a short glyph. |
| `entry` | yes | Path of the JavaScript module that exports `mount`. |
| `styles` | no | Stylesheets the shell loads into the module's shadow root, in order. |
| `home` | no | Module path shown at the module root; defaults to the first `nav` path. |
| `nav` | no | Navigation entries: module path, title, optional icon and group. |

All paths are relative to the assets root. Module paths (`home`, `nav[].path`)
are relative to the module and start with `/`.

## 3. Browser entry point

`entry` is an ES module exporting `mount`:

```js
export function mount(el, ctx) {
  // Render the module into el.
  return {
    update(ctx) { /* the route or another context value changed */ },
    unmount() { /* release listeners, timers, connections */ },
  };
}
```

`ctx` contains:

| Field | Meaning |
|---|---|
| `id` | The module id. |
| `apiBase` | Absolute URL of the module's API, ending with `/`. |
| `assetsBase` | Absolute URL of the module's assets, ending with `/`. |
| `path` | The current module path, e.g. `/containers/abc`. |
| `query` | The current query parameters, as an object. |
| `navigate(path, { replace })` | Go to a module path. |
| `href(path, query)` | The URL of a module path, for `<a href>`. |
| `link(moduleId, path, query)` | URL of a path in another module, or `null` if it is not mounted. |
| `lang` | Interface language, e.g. `zh-CN` or `en`. |

The shell owns the address bar. Modules route on `ctx.path`, change routes
only through `navigate` or `href`, and never read or write `location` for
routing. The shell calls `update` whenever any `ctx` value changes, and
`unmount` before removing the module.

### Isolation and theming

The shell mounts each module into its own **shadow root**. It loads the
manifest's `styles` into that root before calling `mount`, so a module's CSS
cannot affect the shell or other modules, and vice versa. Dialogs and toasts
render inside `el`.

Theme values reach modules through CSS custom properties, which inherit
across the shadow boundary. The shell defines:

```
--panel-bg  --panel-surface  --panel-surface-2  --panel-border
--panel-text  --panel-muted  --panel-primary  --panel-primary-text
--panel-danger  --panel-radius  --panel-font  --panel-mono
```

Modules should derive their own variables from these, with fallbacks, so
they match the shell when mounted and keep their own look standalone:

```css
:root, :host { --primary: var(--panel-primary, #2563eb); }
```

### Standalone

A standalone module serves its own `index.html` from the same assets. It
links the same stylesheets and calls the same `mount` with a context it
builds itself (typically a small hash router and `apiBase: "api/"`). The
manifest is still served and simply unused.

## 4. Storage

All modules share **one SQLite database** that the shell opens (WAL
journaling, busy timeout, foreign keys) and passes to each module that uses
it. A separate database is the exception, kept for modules with a special
need (for example very high write volume), and decided case by case.

- **Tables** are named `<id>_<name>` (for example `acme_certificates`).
  Modules never read or write another module's tables.
- **Migrations** belong to each module. A module records its schema version
  in its own table (conventionally `<id>_schema`) and migrates in `New`.
  Upgrading a module's own earlier, unprefixed database is that module's job.

Who implements a store follows the minimal-dependency principle:

- **A module that uses SQLite on its own** defines its store, ships the
  SQLite implementation and migrations, and takes a `*sql.DB` in `New`.
  Standalone it opens its own file; mounted it receives the shared database.
- **A module that keeps state in files** (records, configuration) defines a
  store interface with a file implementation as its default, and does not
  depend on a database driver. A shell that wants that state in the shared
  database implements the interface (with tables named after the module) and
  passes it in `opts`.
- **Files that must exist on disk** (compose files, checkouts, app data) go
  under a data directory the shell passes in `opts` (`<data>/<id>/`).
- **Bootstrap** values needed before anything opens (listen address, data
  directory, administrator secret) are the shell's flags or environment.
  State that operators change lives in the database and is edited through
  the UI, not in configuration files.

## Versioning

The contract version only increases for incompatible changes. Additive
changes (new optional manifest fields, new `ctx` fields) keep the version.
Modules must ignore unknown `ctx` fields, and shells must ignore unknown
manifest fields.
