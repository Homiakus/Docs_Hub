package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/db/seeder"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database at %s: %v", cfg.DBPath, err)
	}
	defer database.Close()

	fmt.Printf("Seeding demo data into database at: %s...\n", cfg.DBPath)
	if err := seeder.SeedDemo(ctx, database); err != nil {
		log.Fatalf("seeder error: %v", err)
	}

	fmt.Println("Successfully seeded demo spaces, 100 documents, multi-revisions, and demo users!")
	os.Exit(0)
}
