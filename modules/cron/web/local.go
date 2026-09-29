package web

import (
	"context"
	"database/sql"
)

// LocalModule composes the cron repository, scheduler, and HTTP module for a
// process that stores cron data in SQLite and executes jobs locally.
type LocalModule struct {
	*Module
	engine *Engine
}

// NewLocalModule builds the local cron implementation over the supplied
// database. Start it before serving requests and call Close before closing db.
func NewLocalModule(ctx context.Context, db *sql.DB, runner CommandRunner) *LocalModule {
	engine := NewEngine(ctx, NewRepository(db), runner)
	return &LocalModule{Module: New(engine), engine: engine}
}

func (m *LocalModule) Start(ctx context.Context) error { return m.engine.Start(ctx) }
func (m *LocalModule) Close() error                    { return m.engine.Close() }
