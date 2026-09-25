# ESXi module

The module contains the VMware API client in `esxi`, configuration persistence and HTTP/UI service in `web`, and an independent command.

Run the standalone service with:

```sh
go run ./cmd/esxi web serve --listen 127.0.0.1:8080 --db esxi.db
```

The module opens outbound HTTPS connections to configured ESXi hosts. Credentials are stored in the module database and never included in API responses.
