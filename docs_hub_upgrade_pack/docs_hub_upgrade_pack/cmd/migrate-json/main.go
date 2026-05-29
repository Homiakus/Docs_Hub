package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"docs-hub/internal/store"
	_ "modernc.org/sqlite"
)


func main() {
	from := flag.String("from", getenv("DATA_FILE", "storage.json"), "path to current storage.json")
	to := flag.String("to", "docs-hub.db", "path to target SQLite database")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := sql.Open("sqlite", *to)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		log.Fatal(err)
	}

	schema, err := os.ReadFile("internal/store/schema.sql")
	if err != nil {
		// fallback for future embedding layouts
		log.Fatalf("read schema: %v", err)
	}

	for _, stmt := range strings.Split(string(schema), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			log.Fatalf("apply schema statement %q: %v", stmt, err)
		}
	}

	if err := store.ImportJSON(ctx, db, *from); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Imported %s -> %s\n", *from, *to)
}

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}
