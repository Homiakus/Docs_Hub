package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_createsDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	database, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Verify schema_migrations table exists
	var tableName string
	err = database.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'",
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("schema_migrations table not found: %v", err)
	}
	if tableName != "schema_migrations" {
		t.Errorf("unexpected table name: %s", tableName)
	}
}

func TestMigrate_idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	database, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Run migrations again — should be idempotent (no error)
	err = database.Migrate(ctx)
	if err != nil {
		t.Fatalf("second Migrate call failed: %v", err)
	}

	// Run a third time for good measure
	err = database.Migrate(ctx)
	if err != nil {
		t.Fatalf("third Migrate call failed: %v", err)
	}
}

func TestOpen_createsIntermediateDirs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "subdir1", "subdir2", "test.db")

	ctx := context.Background()
	database, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open should create intermediate directories: %v", err)
	}
	defer database.Close()

	// Verify it works
	var count int
	err = database.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
}

func TestOpen_migrationTablesCreated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	database, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	rows, err := database.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' ORDER BY name",
	)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration error: %v", err)
	}

	// Should have schema_migrations + tables from 001_init.sql and 002_admin_categories.sql
	foundSM := false
	for _, tbl := range tables {
		if tbl == "schema_migrations" {
			foundSM = true
			break
		}
	}
	if !foundSM {
		t.Errorf("schema_migrations table not found in: %v", tables)
	}
	if len(tables) < 3 {
		t.Errorf("expected at least 3 tables, got %d: %v", len(tables), tables)
	}
}

func TestOpen_invalidPath(t *testing.T) {
	// Create a regular file where a directory should go
	dir := t.TempDir()
	blockerPath := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blockerPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}

	// Now try to open a DB where blocker is (should fail because MkdirAll
	// needs blocker to be a directory but it's a file)
	dbPath := filepath.Join(blockerPath, "test.db")
	ctx := context.Background()
	_, err := Open(ctx, dbPath)
	if err == nil {
		t.Fatal("expected error when DB path parent is a regular file")
	}
}

func TestOpen_cancelledContext(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so PingContext fails

	_, err := Open(ctx, dbPath)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMigrate_cancelledContext(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First open with a valid context so the DB is created
	database, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Try migrate with a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = database.Migrate(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context during Migrate")
	}
}

func TestDB_Close(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	ctx := context.Background()
	database, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	err = database.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
