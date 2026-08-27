package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/httpapp"
)

// App is the production composition root. It owns infrastructure lifecycle and
// exposes only the assembled HTTP handler to cmd/docshub. SecureAccess and
// future application services are intentionally wired here rather than inside
// transport handlers or the CLI entrypoint.
type App struct {
	database *db.DB
	handler  http.Handler
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o750); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o750); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	server, err := httpapp.New(cfg, database, logger)
	if err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("initialize http app: %w", err)
	}
	return &App{database: database, handler: server.Routes()}, nil
}

func (a *App) Handler() http.Handler {
	if a == nil || a.handler == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "application unavailable", http.StatusServiceUnavailable)
		})
	}
	return a.handler
}

func (a *App) Close() error {
	if a == nil || a.database == nil {
		return nil
	}
	return a.database.Close()
}
