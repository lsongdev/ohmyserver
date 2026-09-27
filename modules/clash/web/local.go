package web

import (
	"context"
	"database/sql"
	"net/http"
)

// LocalModule composes Clash persistence, controller manager, and HTTP handler.
type LocalModule struct {
	repository *Repository
	manager    *Manager
	module     *Module
}

func NewLocalModule(repository *Repository) *LocalModule {
	manager := NewManager(repository)
	return &LocalModule{repository: repository, manager: manager, module: New(manager)}
}

func NewStandaloneLocalModule(ctx context.Context, db *sql.DB) (*LocalModule, error) {
	repository := NewRepository(db)
	if err := repository.Initialize(ctx); err != nil {
		return nil, err
	}
	return NewLocalModule(repository), nil
}

func (m *LocalModule) Repository() *Repository { return m.repository }
func (m *LocalModule) Service() Service        { return m.manager }
func (m *LocalModule) Web() *Module            { return m.module }
func (m *LocalModule) Handler() http.Handler   { return m.module.Handler() }
func (m *LocalModule) StandaloneHandler() http.Handler {
	return m.module.StandaloneHandler()
}
