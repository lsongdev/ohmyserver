# API v1 Reference

The current application is assembled from feature modules. The browser UI uses Hash routes such as `/#/docker/containers`; backend requests use `/v1/<module>/...`. The module ID is the stable API namespace, and resource routes are relative to that namespace.

Authentication is intentionally outside this migration and is not enforced by the current `/v1` router. Do not expose an unauthenticated instance to an untrusted network.

## Module mounts

| Module | API prefix | Main resources / capabilities |
| --- | --- | --- |
| `acme` | `/v1/acme` | Certificates and renewal, CSR generation, providers, accounts, orders, issuances, DNS challenge targets |
| `clash` | `/v1/clash` | Clash servers, proxies, rules, connections, config, events |
| `cron` | `/v1/cron` | Jobs, enable/run actions, execution history |
| `dashboard` | `/v1/dashboard` | `GET /overview` (enabled widget composition plus live local system summary) |
| `dhcp` | `/v1/dhcp` | Settings, leases, static leases, service status/actions |
| `disks` | `/v1/disks` | Filesystem usage, devices, SMART health |
| `dns` | `/v1/dns` | Settings/status, providers/zones/records, local zones, subscriptions, rules |
| `docker` | `/v1/docker` | Containers, logs/stats, images, volumes, networks, Compose, app catalog/install |
| `esxi` | `/v1/esxi` | ESXi server configuration, tests, inventory |
| `files` | `/v1/files` | Directory entries, upload/download, create/delete |
| `forward` | `/v1/forward` | TCP/UDP listeners and lifecycle actions |
| `idrac` | `/v1/idrac` | iDRAC servers, tests, inventory, power |
| `ipmi` | `/v1/ipmi` | IPMI servers, tests, inventory, power |
| `monitor` | `/v1/monitor` | Hosts, current metrics, metric history |
| `ntp` | `/v1/ntp` | Settings, status, service actions |
| `openwrt` | `/v1/openwrt` | Router configuration, status sections, actions |
| `packages` | `/v1/packages` | Package listing/search, refresh, install/remove/upgrade |
| `projects` | `/v1/projects` | Projects, deployments, logs/events, runtime, cancellation, webhook-secret rotation |
| `pve` | `/v1/pve` | Proxmox servers, tests, inventory, VM creation |
| `settings` | `/v1/settings` | Key/value settings CRUD |
| `speedtest` | `/v1/speedtest` | Servers, runs, result history |
| `ssh` | `/v1/ssh` | SSH servers, tests, stats stream, metrics |
| `system` | `/v1/system` | System information and update action |
| `terminal` | `/v1/terminal` | Local, SSH, and Docker WebSocket terminal sessions |
| `uptime` | `/v1/uptime` | Monitors, checks, actions |
| `webserver` | `/v1/webserver` | Listeners, virtual hosts, routes, policies, certificate summaries |
| `widgets` | `/v1/widgets` | Dashboard widget configuration |
| `yeelight` | `/v1/yeelight` | Device discovery/configuration/state/control |

Route patterns and accepted request DTOs are defined by each module's `Handler` and module API tests under [`modules/`](../modules/). The API is not yet a uniform generated OpenAPI contract; response envelopes and operation-specific behavior are documented and tested at module level.

## HTTP behavior

- Unknown module or endpoint returns JSON `404`; an unsupported method returns JSON `405` with normal `Allow` semantics.
- Most JSON reads return `200`; creates generally return `201`; updates/actions return either a JSON result or `204`, depending on the operation. Clients must not parse a body after `204`.
- Errors use a JSON object with an `error` object (`code` and `message`); validation details vary by module.
- Long-lived streams keep their native response type: SSE uses `text/event-stream`; terminal endpoints use WebSocket upgrade. They are not converted into JSON by the module mount layer.
- Terminal WebSocket upgrades reject cross-origin browser requests by default (Origin must match request scheme and host). A reverse-proxy deployment that changes the externally visible origin must inject an explicit origin policy in the host composition; forwarded host headers are not trusted by the module.
- File downloads and icon responses keep their content type and disposition rather than being wrapped in JSON.
- Request paths are same-origin by default. Module API clients use relative paths and receive the mounted API transport from the shell; standalone module handlers provide their own API base.

## ACME renewal

`POST /v1/acme/certificates/{id}/renew` starts a renewal for an active certificate. The JSON body selects a valid account and challenge method:

```json
{"account_id": 3, "challenge_type": "dns-01", "dns_target": "local:example.test"}
```

The response is `202` with the queued issuance. Poll `GET /v1/acme/issuances/{issuance_id}` for progress. A failed renewal leaves the currently active certificate available; only a successfully verified replacement updates it. After the replacement is stored, the host invokes the ACME certificate-update callback so enabled HTTPS listeners reload their configuration.

For the implementation plan and isolation rules, see [the modular frontend/backend plan](frontend-modular-refactor.md). Feature modules share the configured SQLite connection, but own their schemas and repositories; [`internal/architecture`](../internal/architecture/) contains the table-ownership regression test.
