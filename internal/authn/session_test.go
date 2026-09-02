package authn

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
)

type failingRandomReader struct{}

func (failingRandomReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

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

	cookieVal, csrf, exp, err := mgr.CreateSession(ctx, 1, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if cookieVal == "" || csrf == "" || exp == "" {
		t.Fatalf("invalid session outputs: cookie=%q csrf=%q exp=%q", cookieVal, csrf, exp)
	}

	parts := splitTwo(cookieVal, ".")
	var storedIP, storedUA, lastSeen string
	if err := database.QueryRowContext(ctx, `SELECT client_ip,user_agent,last_seen_at FROM sessions WHERE id=?`, parts[0]).Scan(&storedIP, &storedUA, &lastSeen); err != nil {
		t.Fatalf("read session client state: %v", err)
	}
	if storedIP != "127.0.0.1" || storedUA != "test-agent" || lastSeen == "" {
		t.Fatalf("unexpected session client state: ip=%q ua=%q last_seen=%q", storedIP, storedUA, lastSeen)
	}

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

	tampered := cookieVal + "tampered"
	tamperedUser, _, _ := mgr.ValidateSession(ctx, tampered)
	if tamperedUser != nil {
		t.Fatalf("tampered session should not validate")
	}

	inactCookie, _, _, err := mgr.CreateSession(ctx, 2, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create inactive session: %v", err)
	}
	inactUser, _, _ := mgr.ValidateSession(ctx, inactCookie)
	if inactUser != nil {
		t.Fatalf("inactive user session should not validate")
	}

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
	var remaining int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id=?`, parts[0]).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("expired session must be revoked, remaining=%d", remaining)
	}
}

func TestSessionManagerIdleTimeout(t *testing.T) {
	ctx := context.Background()
	database := setupSessionTestDB(t)
	mgr := NewSessionManager(database, "test-super-secret-key-123456789012")
	mgr.idleTimeout = 30 * time.Minute

	cookieVal, _, _, err := mgr.CreateSession(ctx, 1, "127.0.0.1", "idle-agent")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitTwo(cookieVal, ".")
	stale := time.Now().UTC().Add(-31 * time.Minute).Format(time.RFC3339)
	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if _, err := database.ExecContext(ctx, `UPDATE sessions SET last_seen_at=?, expires_at=? WHERE id=?`, stale, future, parts[0]); err != nil {
		t.Fatal(err)
	}

	user, _, err := mgr.ValidateSession(ctx, cookieVal)
	if err != nil {
		t.Fatalf("validate idle-expired session: %v", err)
	}
	if user != nil {
		t.Fatalf("idle-expired session must be rejected")
	}
	var remaining int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id=?`, parts[0]).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("idle-expired session must be revoked, remaining=%d", remaining)
	}
}

func TestSessionManagerTouchesActiveSession(t *testing.T) {
	ctx := context.Background()
	database := setupSessionTestDB(t)
	mgr := NewSessionManager(database, "test-super-secret-key-123456789012")

	cookieVal, _, _, err := mgr.CreateSession(ctx, 1, "127.0.0.1", "touch-agent")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitTwo(cookieVal, ".")
	old := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Second)
	if _, err := database.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE id=?`, old.Format(time.RFC3339), parts[0]); err != nil {
		t.Fatal(err)
	}
	if user, _, err := mgr.ValidateSession(ctx, cookieVal); err != nil || user == nil {
		t.Fatalf("validate active session: user=%v err=%v", user, err)
	}
	var touchedRaw string
	if err := database.QueryRowContext(ctx, `SELECT last_seen_at FROM sessions WHERE id=?`, parts[0]).Scan(&touchedRaw); err != nil {
		t.Fatal(err)
	}
	touched, err := time.Parse(time.RFC3339, touchedRaw)
	if err != nil {
		t.Fatal(err)
	}
	if !touched.After(old) {
		t.Fatalf("last_seen_at was not advanced: old=%s new=%s", old, touched)
	}
}

func TestSessionTouchDoesNotExtendAbsoluteLifetime(t *testing.T) {
	ctx := context.Background()
	database := setupSessionTestDB(t)
	mgr := NewSessionManager(database, "test-super-secret-key-123456789012")

	cookieVal, _, expiresAt, err := mgr.CreateSession(ctx, 1, "127.0.0.1", "absolute-lifetime-agent")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitTwo(cookieVal, ".")
	old := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := database.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE id=?`, old, parts[0]); err != nil {
		t.Fatal(err)
	}

	if user, _, err := mgr.ValidateSession(ctx, cookieVal); err != nil || user == nil {
		t.Fatalf("validate active session: user=%v err=%v", user, err)
	}
	var storedExpiresAt string
	if err := database.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE id=?`, parts[0]).Scan(&storedExpiresAt); err != nil {
		t.Fatal(err)
	}
	if storedExpiresAt != expiresAt {
		t.Fatalf("touch extended absolute lifetime: before=%s after=%s", expiresAt, storedExpiresAt)
	}
}

func TestSessionManagerFailsClosedWhenEntropyUnavailable(t *testing.T) {
	ctx := context.Background()
	database := setupSessionTestDB(t)
	mgr := NewSessionManager(database, "test-super-secret-key-123456789012")
	mgr.random = failingRandomReader{}

	cookieVal, csrf, exp, err := mgr.CreateSession(ctx, 1, "127.0.0.1", "test-agent")
	if err == nil {
		t.Fatalf("expected entropy error")
	}
	if cookieVal != "" || csrf != "" || exp != "" {
		t.Fatalf("failed entropy must not return session material")
	}
	var count int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed entropy must not persist a session, count=%d", count)
	}
}

func TestHashTokenUsesHMACSHA256(t *testing.T) {
	const want = "aa900acb34c6e64089ce061bb6e53053ecc0af1e03fd3a9aa63540d874843147"
	if got := hashToken("token-value", "test-secret"); got != want {
		t.Fatalf("hashToken()=%q want HMAC-SHA256 %q", got, want)
	}
	if tokenHashMatches(want, "different-token", "test-secret") {
		t.Fatal("different token must not match stored HMAC")
	}
	if tokenHashMatches("not-hex", "token-value", "test-secret") {
		t.Fatal("malformed stored token hash must fail closed")
	}
}
