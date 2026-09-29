# moduleweb

Versioned HTTP and browser contract for mount-independent feature modules.

`Wrap(api *http.ServeMux, assets fs.FS)` returns the complete module handler:

- `/`: standalone HTML; use `api-base="."` and `src="assets/index.js"`.
- `/manifest.json`: module identity and presentation metadata.
- `/assets/`: module files; `/assets/runtime/` is reserved for this package.
- Other paths: module API, with JSON errors for unmatched routes/methods.

Run this handler directly or mount it using `http.StripPrefix`. The embedding
host must redirect its bare mount to the trailing-slash root. The handler never
reads forwarded headers, environment mode, a service locator, or a host prefix.
Business errors and streaming responses are passed through unchanged.

`BrowserHandler()` exposes the same embedded runtime to a multi-module shell.
The browser contract is version 1; the Go package dependency is v0.1.0. Module
entrypoints export `{ id, routes, createAPI }`. Hosts use `discoverModule`,
`loadRuntime`, and `createModuleView` from `browser/`. Pages receive `api`,
`params`, `search`, `navigate`, `href`, and `hrefTo`; `hrefTo` returns null when
the target is unavailable. Module code has no standalone/embedded mode flag.

The runtime is plain ESM with no bundler or copied Preact. `createModuleView`
accepts UI primitives from its host. Standalone entries import them directly
from `https://lsong.org/scripts/react/index.js`. Import maps resolve `@module/ui`
and `preact/hooks` to the same Preact instance used by their host renderer.
All HTTP transport capabilities use the host-owned same-origin credential
policy.

Check Go with `GOWORK=off go test ./...` and `GOWORK=off go build ./...`.
Check browser behavior with `node --test browser/*.test.js`.

Before publishing the dependent modules, publish this module as
`github.com/lsongdev/ohmyserver/pkg/moduleweb` v0.1.0. The repository's
`tests/module-smoke/check-runtime.py` verifies the packaged snapshot through a
temporary Go proxy without publishing or relying on workspace/local replaces.
