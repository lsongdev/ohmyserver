package server

import "log"

type (
	InitSchemaHook func(*Server) error
	MigrateHook    func(*Server) error
	InitHook       func(*Server) error
	CloseHook      func(*Server) error
)

var (
	initSchemaHooks []InitSchemaHook
	migrateHooks    []MigrateHook
	initHooks       []InitHook
	closeHooks      []CloseHook
)

func RegisterInitSchemaHook(hook InitSchemaHook) {
	initSchemaHooks = append(initSchemaHooks, hook)
}

func RegisterMigrateHook(hook MigrateHook) {
	migrateHooks = append(migrateHooks, hook)
}

func RegisterInitHook(hook InitHook) {
	initHooks = append(initHooks, hook)
}

func RegisterCloseHook(hook CloseHook) {
	closeHooks = append(closeHooks, hook)
}

func (s *Server) runInitSchemaHooks() error {
	for _, hook := range initSchemaHooks {
		if err := hook(s); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) runMigrateHooks() error {
	for _, hook := range migrateHooks {
		if err := hook(s); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) runInitHooks() {
	for _, hook := range initHooks {
		if err := hook(s); err != nil {
			log.Printf("Warning: init hook failed: %v", err)
		}
	}
}

func (s *Server) runCloseHooks() {
	for _, hook := range closeHooks {
		if err := hook(s); err != nil {
			log.Printf("Close hook error: %v", err)
		}
	}
}
