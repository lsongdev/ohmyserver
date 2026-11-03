// Package notify connects the notification center to the modules that report to
// it: each defines the small interface it needs, and implements it here, with
// the source fixed so that it cannot be forged. The modules never import one
// another.
package notify

import (
	"context"

	notifyapi "github.com/lsongdev/ohmyserver/modules/notify/api"
	zfsapi "github.com/lsongdev/ohmyserver/modules/zfsmgr/api"
)

// ZFS reports the ZFS module's events, under its id.
type ZFS struct{ Service *notifyapi.Service }

var _ zfsapi.Notifier = ZFS{}

func (n ZFS) Notify(ctx context.Context, e zfsapi.Event) error {
	return n.Service.Notify(ctx, "zfs", notifyapi.Event{
		Level: e.Level, Title: e.Title, Body: e.Body, Link: e.Link, Key: e.Key, Resolved: e.Resolved,
	})
}
