# Packages module

This module detects and operates on the host's supported package manager. The
`pkgmgr` package contains the reusable backend clients and parsers; `service`
implements the module's catalog and mutation contract; `web` provides the
relative HTTP API and module-owned ESM assets.

Run the standalone web UI with:

```sh
go run ./cmd/packages web serve --listen 127.0.0.1:8080
```

Package mutations run as the current user on Homebrew, or as root for system
package managers. For non-root users, the module uses non-interactive `sudo`
or `doas`; the host must already allow those commands without prompting.
