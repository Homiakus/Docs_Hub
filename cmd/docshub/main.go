package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/httpapp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	if err := os.MkdirAll(cfg.UploadDir, 0o750); err != nil {
		logger.Error("upload dir", "err", err)
		os.Exit(1)
	}

	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	app, err := httpapp.New(cfg, database, logger)
	if err != nil {
		logger.Error("app init", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: app.Routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("Docs Hub Next started", "addr", cfg.Addr, "db", cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
