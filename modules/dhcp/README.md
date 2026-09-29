# DHCP module

The module owns DHCP settings and lease data, IPv4 validation, DHCP packet handling, address-pool restoration, and the UDP runtime. Embedding hosts provide the service-policy adapter when actions must participate in host-wide listener conflict checks or process lifecycle management.

Run the module independently with:

```sh
go run ./cmd/dhcp web serve --listen 127.0.0.1:8080 --db ./dhcp.db
```

The standalone server mounts the complete module handler at `/`, so its UI, relative API, manifest, and assets share one root. An embedding host may mount the same handler at any prefix. Starting the DHCP server requires permission to bind the configured UDP port and access to the target network.
