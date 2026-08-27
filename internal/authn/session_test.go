package authn

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
)

func setupSessionTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_sessions.db")
	database, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	_, err = database.ExecContext(ctx, `
		INSERT OR IGNORE INTO users(id, username, display_name, email, password_hash, role, is_active, created_at, updated_at)
		VALUES
		(1, 'active_user', 'Active User', 'active@example.com', 'hash', 'admin', 1, datetime('now'), datetime('now')),
		(2, 'inactive_user', 'Inactive User', 'inactive@example.com', 'hash', 'reader', 0, datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("seed users: %v", err)
	}

	return database
}

func TestSessionManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	database := setupSessionTestDB(t)
	mgr := NewSessionManager(database, "test-super-secret-key-123456789012")

	// 1. Create session for active user
	cookieVal, csrf, exp, err := mgr.CreateSession(ctx, 1, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if cookieVal == "" || csrf == "" || exp == "" {
		t.Fatalf("invalid session outputs: cookie=%q csrf=%q exp=%q", cookieVal, csrf, exp)
	}

	// 2. Validate session
	user, validatedCSRF, err := mgr.ValidateSession(ctx, cookieVal)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if user == nil || user.ID != 1 || user.Username != "active_user" {
		t.Fatalf("unexpected user: %v", user)
	}
	if validatedCSRF != csrf {
		t.Fatalf("csrf mismatch: got %q, want %q", validatedCSRF, csrf)
	}

	// 3. Reject tampered token
	tampered := cookieVal + "tampered"
	tamperedUser, _, _ := mgr.ValidateSession(ctx, tampered)
	if tamperedUser != nil {
		t.Fatalf("tampered session should not validate")
	}

	// 4. Inactive user session cannot validate
	inactCookie, _, _, err := mgr.CreateSession(ctx, 2, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create inactive session: %v", err)
	}
	inactUser, _, _ := mgr.ValidateSession(ctx, inactCookie)
	if inactUser != nil {
		t.Fatalf("inactive user session should not validate")
	}

	// 5. Revoke single session
	parts := splitTwo(cookieVal, ".")
	if err := mgr.RevokeSession(ctx, parts[0]); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	revokedUser, _, _ := mgr.ValidateSession(ctx, cookieVal)
	if revokedUser != nil {
		t.Fatalf("revoked session should not validate")
	}
}

func TestSessionManagerExpiration(t *testing.T) {
	ctx := context.Background()
	database := setupSessionTestDB(t)
	mgr := NewSessionManager(database, "test-super-secret-key-123456789012")

	cookieVal, _, _, err := mgr.CreateSession(ctx, 1, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	parts := splitTwo(cookieVal, ".")

	// Expire session manually in DB
	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	if _, err := database.ExecContext(ctx, `UPDATE sessions SET expires_at=? WHERE id=?`, past, parts[0]); err != nil {
		t.Fatalf("update expires_at: %v", err)
	}

	user, _, err := mgr.ValidateSession(ctx, cookieVal)
	if err != nil {
		t.Fatalf("validate expired session: %v", err)
	}
	if user != nil {
		t.Fatalf("expired session must return nil user")
	}
}
