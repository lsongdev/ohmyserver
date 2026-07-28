# DNS Server

OhMyServer embeds the [`github.com/lsongdev/dns-go`](https://github.com/lsongdev/dns-go)
resolution pipeline. DNS data and runtime settings are stored in the main
OhMyServer SQLite database and managed from `/dns`.

## Resolution order

Queries pass through these stages:

1. response cache;
2. local DNS zones and records;
3. optional allow/block rules and subscriptions;
4. upstream resolvers in failover order.

Filtering is optional. The DNS server can be used only for local authoritative
records, only as a forwarding resolver, or with both features enabled.

## Listeners

UDP, TCP, DNS-over-TLS (DoT), and DNS-over-HTTPS (DoH) listeners are configured
independently. The default address for UDP and TCP is `:15353`, but the server is
disabled until it is explicitly enabled. Binding port 53 may require elevated
privileges or an operating-system capability.

DoT requires a PEM certificate and private key. DoH accepts RFC 8484 GET and
POST requests at `/dns-query`. Its embedded listener is plain HTTP so it can be
placed behind OhMyServer's web server or another TLS reverse proxy.

## Upstreams

Enter one upstream per line:

```text
udp 1.1.1.1:53 5s
tcp 8.8.8.8:53 5s
dot dns.example.com:853 5s
doh https://dns.example.com/dns-query 5s post
```

The format is `type address [timeout] [method]`. The method is used only by
DNS-over-HTTPS and defaults to `post`.

## Local records

Zones can contain `A`, `AAAA`, `CNAME`, `MX`, `NS`, `TXT`, `PTR`, `SOA`, and
`SRV` records. Record names are relative to the zone; use `@` for the zone
apex.

## Hosted DNS providers

`/dns/providers` manages connections to Cloudflare, DNSPod, and Name.com.
Connection detail pages load hosted zones and records from the provider API.
The former `/domains` namespace is not restored.

The database deliberately distinguishes:

- `dns_zones` and `dns_server_records`, served by the embedded DNS runtime;
- `dns_provider_connections`, whose zones and records are managed through a
  remote provider API.

Both local and hosted zones are offered as DNS-01 targets by the ACME
certificate form. The user explicitly selects the target; OhMyServer verifies
that every requested certificate name belongs to the selected zone.

## ACME DNS-01

When a certificate uses DNS-01, OhMyServer creates the exact required TXT
record in the selected zone, asks the ACME server to validate it, and removes
only the record created by that issuance. Local DNS changes reload the embedded
runtime. Provider-backed changes are applied and cleaned up through the
provider API.

## Filters and subscriptions

Custom filters use dns-go's DNS-oriented AdBlock syntax, for example:

```text
||telemetry.example^
@@||required.example^
```

Remote block and allow subscriptions are cached under the directory containing
the OhMyServer database. Enabled subscriptions are checked every 30 minutes and
downloaded when their configured refresh interval has elapsed. Downloads are
limited to 32 MiB; a failed refresh keeps the last successful local copy.
