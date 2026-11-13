# Xray

OhMyServer links [Xray-core](https://github.com/XTLS/Xray-core) into the Go
binary. The module uses the same engine, HTTP API, and UI in both hosts.

- In OhMyServer, open **Xray** in the navigation. State lives under
  `<dataDir>/xray`; the management API uses the panel's authentication.
- To run on its own, run `make deps` from the repository root, then
  `go run ./modules/xray/cmd/xray-go -listen 127.0.0.1:8090 -d ./xray-data`.
  Set `XRAY_GO_PASSWORD` for HTTP Basic authentication (any username).
  An address outside loopback requires a password; terminate TLS in
  front of the management listener if accessing it over a network.

The module starts stopped on a fresh install. Create or edit entries on
**Inbounds**, **Outbounds**, and **Rules**; outbound and rule order is
editable. Use **Configuration** to paste a full Xray JSON document or
edit settings such as DNS and routing strategy. GeoIP and GeoSite
references require the corresponding Xray data files; set
`XRAY_LOCATION_ASSET` for the service to point at their directory.

Saving compiles the draft with Xray's configuration builder. **Start Xray**
or **Apply changes** on Overview starts the saved draft. Applying a new
configuration restarts the embedded core and may interrupt connections.
If startup fails, the module attempts to restore the last running
configuration. Restarting the host loads the last applied configuration;
**Stop Xray** disables that startup without discarding the draft.

The module serves `GET /config`, `PUT /config`, `POST /apply`, and
`POST /stop` relative to its API base. Writes take the current `revision`
from GET; PUT also takes a `config` object. A stale revision returns 409.
Use separate state directories and data-plane ports for independent hosts.
