package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
)

func TestNewBuildsProductionHandlerAndInfrastructure(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DBPath:        filepath.Join(dir, "data", "docshub.db"),
		UploadDir:     filepath.Join(dir, "uploads"),
		SiteName:      "Bootstrap Test",
		AdminUser:     "admin",
		AdminPassword: "bootstrap-password",
		SessionSecret: "bootstrap-session-secret-32-bytes",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := New(context.Background(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	if _, err := os.Stat(cfg.UploadDir); err != nil {
		t.Fatalf("upload dir not created: %v", err)
	}
	if _, err := os.Stat(cfg.DBPath); err != nil {
		t.Fatalf("database not created: %v", err)
	}

	server := httptest.NewServer(app.Handler())
	defer server.Close()
	res, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("healthz status=%d want %d", res.StatusCode, http.StatusOK)
	}
}

func TestNilAppHandlerFailsClosed(t *testing.T) {
	var app *App
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestSeedDemoOwnsPersistenceLifecycle(t *testing.T) {
	t.Setenv("DEMO_PASSWORD", "demo-password-123")
	ctx := context.Background()
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "seed", "docshub.db")}
	if err := SeedDemo(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var articles int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM articles WHERE deleted_at IS NULL`).Scan(&articles); err != nil {
		t.Fatal(err)
	}
	if articles == 0 {
		t.Fatal("seed demo produced no articles")
	}
}
