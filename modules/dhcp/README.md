# DHCP module

The module owns DHCP settings and lease data, IPv4 validation, DHCP packet handling, address-pool restoration, and the UDP runtime. Embedding hosts provide the service-policy adapter when actions must participate in host-wide listener conflict checks or process lifecycle management.

Run the module independently with:

```sh
go run ./cmd/dhcp web serve --listen 127.0.0.1:8080 --db ./dhcp.db
```

The standalone server provides a management UI at `/` and the API plus module manifest under `/v1/dhcp`. Starting the DHCP server requires permission to bind the configured UDP port and access to the target network.
