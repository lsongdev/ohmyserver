# notify

The notification center: modules report what happens, it keeps the events,
tracks the problems still open, and sends them to chat services. OhMyServer's
overview shows the open ones. See also
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

## Channels

A channel is somewhere messages go: a Feishu or WeCom group bot, Bark, ntfy, any
web hook (as JSON), or Telegram. Each has a rule, the lowest level it wants and
optionally which sources; by default warnings and above from anywhere, so that
adding one needs no more than its address. A resolved problem is announced
wherever the problem was sent, and nowhere else.

Sending goes through an outbox, written with the event, so a restart loses
nothing and reporting never waits for it. A message could be sent twice if the
panel stops at the very moment it went out.

- A channel sends strictly in order: what comes later waits behind a failed
  message, since a stale "resolved" after a newer "opened" would mislead.
- A failed send retries after 30 seconds, 2 minutes, 10 minutes, then hourly, and
  is given up on after a day, when it can be sent again by hand from the log.
- A channel gets at most 20 messages a minute. More than that waiting at once
  becomes one summary, most severe first.
- A muted issue is recorded but not sent, including if it gets worse.
- A channel failing shows on its status, and never produces an event, or a dead
  channel would feed itself.

Addresses and tokens are secrets, since a bot's address is all it takes to post
as it. The API never returns them, errors are scrubbed of them, and redirects are
not followed. They are stored in the database as other modules' credentials are,
in plain text.

## API

Responses are `{"data": …}`; errors are `{"error": {"code", "message"}}`.

| Method | Path | |
|---|---|---|
| GET | `/events` | Newest first: `level`, `source`, `q`, `before`, `limit` |
| GET | `/sources` | Who has sent events |
| GET | `/issues` | `state=open` (the default) or `resolved` |
| POST | `/issues/{id}/resolve` | Close by hand |
| POST | `/issues/{id}/mute` | `{minutes}`; 0 ends it |
| GET, POST | `/channels` | `{name, type, preset, url, token, chat_id, min_level, sources, enabled}` |
| GET, PUT, DELETE | `/channels/{id}` | On PUT, an empty `url` or `token` keeps the stored one |
| POST | `/channels/{id}/test` | Sends a test message; 502 `send_failed` says why not |
| GET | `/deliveries` | The log, newest first: `channel`, `state`, `before` |
| POST | `/deliveries/{id}/retry` | Queue one that was given up on |
| GET | `/info` | Whether the panel has no sign-in |
| GET | `/summary` | Counts and the most severe open issues |
