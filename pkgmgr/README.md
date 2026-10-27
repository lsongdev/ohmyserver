# pkgmgr

Manage a Linux server's packages from the browser: list what is installed,
check for and apply updates, search the repositories, install and remove
packages, and install Docker Engine the way Docker recommends.

It drives the system's own package manager: **dnf** (Fedora, RHEL, Rocky,
AlmaLinux, Alibaba Cloud Linux, …) or **apt** (Debian, Ubuntu). Changes run
as jobs, one at a time, and the UI follows their output. A package
transaction is never interrupted: stopping the server waits for the running
job.

This module (id `packages`, shown as Packages) is built into OhMyServer for
now, and will become a project of its own once it matures. It already
follows the [module contract](../docs/module-contract.md) and imports nothing
else from this repository, so moving it out is a matter of copying this
directory and adding a go.mod.

## Run

```sh
go install github.com/lsongdev/ohmyserver/pkgmgr/cmd/pkgmgr@latest
sudo PKGMGR_PASSWORD=secret pkgmgr -listen 127.0.0.1:8080
```

Installing and removing packages needs root. Without `PKGMGR_PASSWORD` it
serves without authentication, which is only safe on a loopback address.

## API

Responses are `{"data": …}`, errors `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET | `/api/system` | Package manager, distribution and the running job |
| GET | `/api/installed` | Installed packages |
| GET | `/api/updates` | Available updates, from the cached package lists |
| GET | `/api/search?q=nginx` | Available packages whose name contains `q` |
| GET | `/api/info?name=nginx` | The package manager's description of a package |
| GET | `/api/docker` | Whether Docker is installed and running, and where it can come from |
| POST | `/api/jobs` | Start a job: `{"action": "install" \| "remove" \| "upgrade", "packages": […]}`, `{"action": "refresh"}`, or `{"action": "docker", "source": "docker" \| "aliyun"}` |
| GET | `/api/jobs`, `/api/jobs/{id}` | Jobs since the server started; one job includes its output |

`upgrade` without packages upgrades everything. Starting a job while another
runs answers 409.

## Docker

The Docker recipe adds Docker's repository (from download.docker.com, or the
Alibaba Cloud mirror for hosts in mainland China), installs Docker Engine
with the Buildx and Compose plugins, and enables the service. On Enterprise
Linux derivatives it uses the compatible EL release (Alibaba Cloud Linux 3
uses EL8 packages).
