# Yeelight module

The module manages Yeelight devices over the LAN, including discovery, state reads, and power/brightness/color controls.

```sh
go run ./cmd/yeelight web serve --listen 127.0.0.1:8080 --db yeelight.db
```

Discovery and device control require network access to Yeelight devices. Devices must have LAN control enabled.
