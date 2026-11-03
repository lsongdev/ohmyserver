# notify

The notification center: modules and scripts report what happens, it keeps the
events and tracks the problems still open. OhMyServer's overview shows the open
ones. Sending them to chat and email is next; see
[the design](../../docs/notification-center-design.md).

This module (id `notify`, shown as Notifications) follows the
[module contract](../../docs/module-contract.md) and imports nothing else from
this repository. It runs on its own too: `make deps`, then
`go run ./modules/notify/cmd/notify-go`.

## Reporting

```go
service.Notify(ctx, "zfs", api.Event{Level: "critical", Title: "Pool data is degraded", Key: "pool:data:health", Link: "/pool?name=data"})
service.Notify(ctx, "zfs", api.Event{Key: "pool:data:health", Resolved: true})
```

Without a key, an event is a one-off message. With one, it is the state of a
problem, and a source may repeat it on every check, so it has to remember
nothing:

| Report | Happens |
|---|---|
| A new problem, or one back after the window | The issue opens, with one event |
| A problem going on | It is counted; an event only if its level rose |
| Resolved | Closed at once; its event waits five minutes |
| Back within those five minutes | Open again, with no event, so flapping stays quiet |
| Resolved with nothing open | Nothing |

An issue can also be closed by hand, for a source that is gone. If its source
reports it again, it reopens.

Events are kept for 7 days (debug), 30 (info), 90 (warning) or a year
(critical), at most 100,000 in all, and those of open issues always.

## API

Responses are `{"data": …}`; errors are `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET | `/events` | Newest first: `level`, `source`, `q`, `before`, `limit` |
| GET | `/sources` | Who has sent events |
| GET | `/issues` | `state=open` (the default) or `resolved` |
| POST | `/issues/{id}/resolve` | Close by hand |
| GET | `/summary` | Counts and the most severe open issues |
