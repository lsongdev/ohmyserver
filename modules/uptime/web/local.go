package web

import (
	"net/http"

	uptime "github.com/lsongdev/ohmyserver/modules/uptime"
)

// LocalModule composes uptime persistence, checker, and HTTP handler.
type LocalModule struct {
	repository *Repository
	checker    *uptime.CheckEngine
	module     *Module
}

func NewLocalModule(repository *Repository) *LocalModule {
	checker := uptime.NewChecker(repository)
	return &LocalModule{repository: repository, checker: checker, module: New(repository, checker)}
}

func (m *LocalModule) Repository() *Repository { return m.repository }
func (m *LocalModule) Checker() uptime.Checker { return m.checker }
func (m *LocalModule) Web() *Module            { return m.module }
func (m *LocalModule) Handler() http.Handler   { return m.module.Handler() }
