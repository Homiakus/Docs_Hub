package bootstrap

import (
	"context"
	"fmt"

	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/db/seeder"
)

// SeedDemo keeps infrastructure ownership out of cmd/docshub. CLI commands
// request an application operation; bootstrap decides how persistence is
// constructed and closed.
func SeedDemo(ctx context.Context, cfg config.Config) error {
	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()
	if err := seeder.SeedDemo(ctx, database); err != nil {
		return fmt.Errorf("seed demo: %w", err)
	}
	return nil
}
