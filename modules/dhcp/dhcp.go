package dhcp

import (
	"context"
	"database/sql"
	"github.com/lsongdev/dhcp-go/web"
)

type Options = web.Options

// New mounts DHCP using the panel's shared database. The caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*web.DHCP, error) {
	st, err := NewStore(ctx, db)
	if err != nil {
		return nil, err
	}
	return web.New(ctx, st, opts)
}
