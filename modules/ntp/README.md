# NTP module

The module owns NTP settings, UDP server runtime, HTTP API, and its browser UI. The standalone command starts the management UI; enable and start the NTP listener separately from the UI. Binding the default NTP port (`:123`) may require elevated privileges.

```sh
go run ./cmd/ntp web serve --listen 127.0.0.1:8080 --db ntp.db
```

Embedding hosts may inject their service-policy adapter to coordinate NTP lifecycle with other managed listeners.
