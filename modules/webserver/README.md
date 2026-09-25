# Web Server module

The module owns Web Server configuration, its SQLite repository, the yabber
runtime, the management API, and browser assets. Embedding hosts may provide a
certificate source and ACME HTTP-01 token store through the module interfaces.

Run the independent HTTP management service and configured HTTP listeners with:

```sh
go run ./cmd/webserver web serve --listen 127.0.0.1:8080
```

The standalone command uses a module-owned SQLite database under the user's
configuration directory. It has no ACME certificate provider, so HTTPS
listeners that reference certificates require an embedding host to provide a
`CertificateSource`. The shell injects its ACME adapter and shared service
startup policy while reusing the same `LocalService` and `Runtime`.
