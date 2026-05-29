package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/store"
)

func main() {
	from := flag.String("from", getenv("DATA_FILE", "./data/storage.json"), "path to legacy storage.json")
	to := flag.String("to", getenv("DB_PATH", "./data/docshub.db"), "path to target SQLite database")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := os.MkdirAll(filepath.Dir(*to), 0o750); err != nil {
		log.Fatal(err)
	}
	database, err := db.Open(ctx, *to)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	if err := store.ImportJSON(ctx, database.DB, *from); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Imported %s -> %s\n", *from, *to)
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
