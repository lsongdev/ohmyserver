# iDRAC module

This module manages Dell iDRAC servers over Redfish, including inventory and power control. Credentials are stored in the module database and omitted from API responses.

```sh
go run ./cmd/idrac web serve --listen 127.0.0.1:8080 --db idrac.db
```

The standalone process needs network access to the configured iDRAC endpoints.
