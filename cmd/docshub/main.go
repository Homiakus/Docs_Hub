package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/homiakus/docshub-next/internal/bootstrap"
	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/db/seeder"
)

const Version = "v0.4.0-alpha.2"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("Docs_Hub", Version)
			os.Exit(0)
		case "healthcheck":
			runHealthcheck(os.Args[2:])
			return
		case "seed-demo":
			runSeedDemo()
			return
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load .env file (ignore error if file doesn't exist — env vars can come from system/docker)
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("config validation failed", "err", err)
		os.Exit(1)
	}

	level := parseLogLevel(cfg.LogLevel)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	app, err := bootstrap.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("app init", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := app.Close(); err != nil {
			logger.Error("app close", "err", err)
		}
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("Docs Hub Next started", "version", Version, "addr", cfg.Addr, "db", cfg.DBPath, "tls", cfg.TLS.Enabled, "log_level", cfg.LogLevel)
		if cfg.TLS.Enabled {
			errCh <- srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		} else {
			errCh <- srv.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
	logger.Info("server stopped")
}

func runHealthcheck(args []string) {
	fs := flag.NewFlagSet("healthcheck", flag.ExitOnError)
	targetURL := fs.String("url", "http://127.0.0.1:8080/healthz", "Healthcheck target URL")
	_ = fs.Parse(args)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(*targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Healthcheck failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Healthcheck HTTP status: %d\n", resp.StatusCode)
		os.Exit(1)
	}
	os.Exit(0)
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func runSeedDemo() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	fmt.Printf("Seeding demo data into DB at: %s...\n", cfg.DBPath)
	if err := seeder.SeedDemo(ctx, database); err != nil {
		fmt.Fprintf(os.Stderr, "seeder error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Demo seeding completed successfully!")
	os.Exit(0)
}
