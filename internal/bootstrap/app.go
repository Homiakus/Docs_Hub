package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/httpapp"
	"github.com/homiakus/docshub-next/internal/telegram"
)

// App is the production composition root. It owns infrastructure lifecycle and
// exposes only the assembled HTTP handler to cmd/docshub. SecureAccess and
// future application services are intentionally wired here rather than inside
// transport handlers or the CLI entrypoint.
type App struct {
	database    *db.DB
	handler     http.Handler
	telegramBot *telegram.Bot
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

	var bot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		bot = telegram.NewBot(cfg, database, logger)
		bot.Start(ctx)
		server.SetTelegramBot(bot)
	}

	return &App{database: database, handler: server.Routes(), telegramBot: bot}, nil
}

func (a *App) Handler() http.Handler {
	if a == nil || a.handler == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "application unavailable", http.StatusServiceUnavailable)
		})
	}
	return a.handler
}

func (a *App) GenerateAdminMagicLink(ctx context.Context, username string) (string, error) {
	if a == nil || a.database == nil {
		return "", errors.New("app unavailable")
	}
	if a.telegramBot != nil {
		return a.telegramBot.CreateMagicLink(ctx, username)
	}

	if username == "" {
		username = "admin"
	}
	var userID int64
	var isActive int
	err := a.database.QueryRowContext(ctx, `SELECT id, is_active FROM users WHERE LOWER(username)=LOWER(?)`, username).Scan(&userID, &isActive)
	if err != nil {
		return "", err
	}
	if isActive == 0 {
		return "", errors.New("user is blocked")
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	rawToken := hex.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute).Format(time.RFC3339)
	createdAt := now.Format(time.RFC3339)

	_, err = a.database.ExecContext(ctx,
		`INSERT INTO auth_tokens(token_hash, user_id, expires_at, created_at) VALUES(?, ?, ?, ?)`,
		tokenHash, userID, expiresAt, createdAt,
	)
	if err != nil {
		return "", err
	}

	return rawToken, nil
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.telegramBot != nil {
		a.telegramBot.Stop()
	}
	if a.database == nil {
		return nil
	}
	return a.database.Close()
}
