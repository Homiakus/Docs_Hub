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

	// Should have schema_migrations + tables from embedded migrations.
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

func TestOpen_domainsProjectsCompatibilityMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	ctx := context.Background()

	database, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	var (
		domainID        int64
		domainStableKey string
		domainSlug      string
		domainStatus    string
	)
	if err := database.QueryRowContext(ctx, `
		SELECT id, stable_key, slug, status
		FROM domains
		WHERE organization_id = 1
		ORDER BY id
		LIMIT 1`).Scan(&domainID, &domainStableKey, &domainSlug, &domainStatus); err != nil {
		t.Fatalf("query compatibility domain: %v", err)
	}
	if domainID == 0 || domainStableKey != "legacy-domain-1" || domainSlug != "general" || domainStatus != "active" {
		t.Fatalf("unexpected compatibility domain: id=%d key=%q slug=%q status=%q", domainID, domainStableKey, domainSlug, domainStatus)
	}

	var (
		projectDomainID int64
		projectStable   string
		accessMode      string
		projectStatus   string
	)
	if err := database.QueryRowContext(ctx, `
		SELECT domain_id, stable_key, access_mode, status
		FROM spaces
		WHERE id = 1`).Scan(&projectDomainID, &projectStable, &accessMode, &projectStatus); err != nil {
		t.Fatalf("query compatibility project: %v", err)
	}
	if projectDomainID != domainID {
		t.Fatalf("legacy project domain_id=%d want %d", projectDomainID, domainID)
	}
	if projectStable != "legacy-project-1" {
		t.Fatalf("legacy project stable_key=%q", projectStable)
	}
	if accessMode != "inherit" || projectStatus != "active" {
		t.Fatalf("legacy project access/status = %q/%q", accessMode, projectStatus)
	}

	var migrationCount int
	if err := database.QueryRowContext(ctx, `
		SELECT count(*) FROM schema_migrations
		WHERE version = '008_domains_projects_compat.sql'`).Scan(&migrationCount); err != nil {
		t.Fatalf("query migration marker: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration marker count=%d want 1", migrationCount)
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
