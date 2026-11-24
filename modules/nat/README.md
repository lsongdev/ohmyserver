# NAT traversal

An independently runnable module (id `nat`) for discovering the network path
from the server. The first version measures **IPv4 UDP NAT behavior**. It does
not establish tunnels, test TCP, verify IPv6 inbound access, or demonstrate that
a particular peer can connect.

The module has its own API, embedded UI and standalone command. It does not
use NetworkManager, iproute2, nftables or forward-go. OhMyServer places its menu
under Network, alongside network management, firewall and port forwarding.

## Detection

- Binding requests to configurable STUN servers share one UDP socket, so their
  observed public endpoints can be compared. A selected local IPv4 address is
  bound directly; it does not override routing or proxy policy.
- Full RFC 5780 discovery requires RESPONSE-ORIGIN and OTHER-ADDRESS advertising
  two IPv4 addresses and two ports. The origin is contacted directly and its
  alternate endpoint are verified on a fresh socket before behavior tests begin.
  This avoids reusing Fake-IP session state from the initial binding tests.
- Mapping tests compare the original destination with the alternate IP, and
  when necessary the alternate IP and port.
- Filtering tests start on a **new socket**, before contacting any alternate
  endpoint. Replies must match the transaction ID and the expected source.
  CHANGE-REQUEST timeouts are retried and only interpreted with successful
  control requests to the alternate endpoints using the original mapping.
- Filtering inferred from timeouts remains subject to server implementation,
  packet loss, proxies and firewalls. The report includes the individual steps
  and these limitations. Unsupported servers and failed controls leave the
  affected behavior unknown; a failed binding alone does not prove UDP blocking.
- Stable mappings across ordinary servers are an observation, distinct from
  a completed behavior-discovery test. Traditional cone/symmetric descriptions
  are secondary to mapping and filtering results, and do not use NAT1–NAT4 labels.
- DNS answers within 198.18.0.0/15 are flagged as possible Fake-IP. No resolver,
  proxy, firewall or network settings are changed.

Each transaction retries once with a one-second response window. A whole check
has a 60-second limit. One check runs at a time; leaving or refreshing its page
does not stop it. Up to 20 checks remain in memory until process restart.
Reports can be downloaded as JSON. Stopping a check preserves its evidence.

References: [STUN](https://www.rfc-editor.org/rfc/rfc8489.html),
[RFC 5780 behavior discovery](https://www.rfc-editor.org/rfc/rfc5780.html).

## Standalone

From the repository root:

```sh
make deps
go build -o /tmp/nat-go ./modules/nat/cmd/nat-go
/tmp/nat-go -listen 127.0.0.1:8093
```

No root privileges are required. `NAT_PASSWORD` enables HTTP Basic
authentication (any username), and is required for a non-loopback management
listener. Use HTTPS termination when accessing that listener over a network.
The shared `panel` package supplies browser infrastructure; the detection
engine has no dependency on the OhMyServer shell or other business modules.

## API

Paths below are relative to `/api/` standalone, or `/m/nat/api/` in OhMyServer.
Success responses use `{"data": ...}`, errors `{"error": {"code", "message",
"fields"}}`. The shell owns authentication when mounted.

| Method | Path | Description |
|---|---|---|
| GET | `/defaults` | Default STUN servers and local IPv4 addresses |
| POST | `/checks` | Start `{servers: ["host:port"], local_address?: "IPv4"}`; returns 202 |
| GET | `/checks` | Recent checks, newest first, including progress and results |
| GET | `/checks/{id}` | Inspect a check after refresh or navigation |
| POST | `/checks/{id}/cancel` | Cancel an active check without clearing evidence |

Validation accepts one to five IPv4 STUN endpoints. Duplicate submissions while
any check is active return 409. Malformed settings return 422 with field errors.
Checks never open a local service port for remote application access.
