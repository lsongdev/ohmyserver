package server

import (
	"log/slog"
	"os"
)

func (s *Server) InitSlog() error {
	cfg := s.config.Logger
	level := slog.LevelInfo
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	if cfg.Output != "" {
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			switch cfg.Format {
			case "json":
				handler = slog.NewJSONHandler(f, opts)
			default:
				handler = slog.NewTextHandler(f, opts)
			}
		}
	}

	slog.SetDefault(slog.New(handler))
	return nil
}

func init() {
	RegisterInitHook((*Server).InitSlog)
}
