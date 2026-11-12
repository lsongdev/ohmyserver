# domains

Domains hosted at DNS providers, managed from the browser: provider
accounts (Cloudflare, DNSPod and Name.com), the zones each holds, and their records:
list, filter, add, edit and delete, with Cloudflare's proxy switch.

It also publishes ACME DNS-01 challenges in those zones (`Challenges`),
so a certificate manager can issue certificates for them, wildcards
included, without opening a port. In OhMyServer the certificate manager
tries the DNS server's own zones first, then these.

This module (id `domains`, shown as Domains) is built into OhMyServer for
now. It follows the [module contract](../../docs/module-contract.md) and
imports nothing else from this repository.

## Providers

Each provider is a subpackage of `provider` registering a `Kind` with the
credentials it needs. The interface is small: list zones, list records
(optionally by type and name), create, update, delete. Providers talk to
the services' REST APIs directly, without vendor SDKs.

| Provider | Credentials |
|---|---|
| Cloudflare | `api_token` with Zone › Zone › Read and Zone › DNS › Edit (the "Edit zone DNS" template), or `email` and `api_key` (Global API Key). A token takes precedence when set. |
| DNSPod | `token` in `ID,Token` format from a main account's DNSPod API token page; uses the traditional mainland API, not Tencent Cloud SecretId/SecretKey |
| Name.com | `user` (account username) and `token` (production API token), from the account's API settings |

Every provider supports zone and record pagination, record creation, updates,
deletion, exact type/name filtering, and ACME DNS-01 TXT publication and cleanup.
Record names in the module API are always absolute; each provider translates
them to its own format. Cloudflare supports automatic TTL and editable comments.
Name.com uses a minimum TTL of 300 seconds; DNSPod uses the domain's TTL floor
(600 seconds when unavailable). Shorter TTLs, including DNS-01's requested TTL,
are raised to the provider's minimum. The record editor shows provider-specific
types, TTL choices and comment support. Name.com also supports ANAME records.

DNSPod record remarks are displayed and preserved but are not edited here.
Updates preserve the record's DNSPod line, enabled state and load-balancing
weight. Name.com has no record comments. Neither provider has Cloudflare's
proxy switch.

API references: [Cloudflare](https://developers.cloudflare.com/api/resources/dns/subresources/records/),
[DNSPod](https://docs.dnspod.cn/api/record-list/),
[Name.com v4](https://www.name.com/api-docs/dns).

Credentials are checked (by listing the zones) before they are saved, and
stored in the SQLite database as given: protect the database file. They
are never sent back to the browser; an empty field on update keeps the
saved value.

## Run

```sh
make deps     # at the repository root: the UI's browser dependencies
go build -o domains ./modules/domains/cmd/domains
DOMAINS_PASSWORD=secret ./domains -listen 127.0.0.1:8080
```

## API

Responses are `{"data": …}`, errors `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET | `/api/kinds` | Supported providers and their credential fields |
| GET/POST | `/api/accounts` | Accounts (`name`, `kind`, `credentials`); secrets are never returned, `set` says which are |
| POST | `/api/accounts/test` | Check credentials without saving; `id` fills empty secrets from that account |
| GET/PUT/DELETE | `/api/accounts/{id}` | |
| GET | `/api/zones` | Every account's zones (cached ten minutes; `?refresh=1`) |
| GET | `/api/accounts/{id}/zones/{zone}` | A zone with its records |
| POST | `/api/accounts/{id}/zones/{zone}/records` | Add a record; names are relative to the zone (`www`, `@`) or absolute |
| PUT/DELETE | `/api/accounts/{id}/zones/{zone}/records/{record}` | |
| GET | `/api/summary` | Counts for an overview |
