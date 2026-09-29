package web

import (
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

func (m *LocalModule) Repository() *Repository { return m.repository }
func (m *LocalModule) Service() Service        { return m.manager }
func (m *LocalModule) Web() *Module            { return m.module }
func (m *LocalModule) Handler() http.Handler   { return m.module.Handler() }
