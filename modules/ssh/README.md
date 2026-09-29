# SSH module

The SSH module provides a reusable SSH client, saved-server and key management, remote metrics, and interactive sessions. `web/` owns persistence, HTTP routes, and embedded browser assets; the standalone command uses the same web service as embedding hosts.

Run it independently with:

```sh
go run ./cmd/ssh web serve --listen 127.0.0.1:8080 --db ./ssh.db
```

The standalone process mounts the complete module handler at `/`; embedding hosts may mount the same handler at any prefix. Hosts that also expose terminal access can connect the module's session capability to the shared terminal transport package.
