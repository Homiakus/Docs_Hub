package authn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
)

type SessionManager struct {
	db            *db.DB
	sessionSecret string
	idleTimeout   time.Duration
	maxLifetime   time.Duration
}

func NewSessionManager(d *db.DB, secret string) *SessionManager {
	return &SessionManager{
		db:            d,
		sessionSecret: secret,
		idleTimeout:   24 * time.Hour,
		maxLifetime:   7 * 24 * time.Hour,
	}
}

func (s *SessionManager) CreateSession(ctx context.Context, userID int64, ip, userAgent string) (string, string, string, error) {
	sid := randomString(24)
	token := randomString(32)
	csrf := randomString(32)

	now := time.Now().UTC()
	expiresAt := now.Add(s.maxLifetime).Format(time.RFC3339)
	createdAt := now.Format(time.RFC3339)
	tokenHash := hashToken(token, s.sessionSecret)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions(id, token_hash, user_id, csrf_token, expires_at, created_at) VALUES(?,?,?,?,?,?)`,
		sid, tokenHash, userID, csrf, expiresAt, createdAt)
	if err != nil {
		return "", "", "", err
	}

	cookieValue := sid + "." + token
	return cookieValue, csrf, expiresAt, nil
}

func (s *SessionManager) ValidateSession(ctx context.Context, cookieValue string) (*domain.User, string, error) {
	if cookieValue == "" {
		return nil, "", nil
	}
	parts := splitTwo(cookieValue, ".")
	if len(parts) != 2 {
		return nil, "", nil
	}
	sid, token := parts[0], parts[1]

	var u domain.User
	var storedHash, exp, csrf string
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.display_name, coalesce(u.email,''), u.role, u.is_active, s.token_hash, s.expires_at, s.csrf_token FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=? AND u.is_active=1`,
		sid).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Active, &storedHash, &exp, &csrf)
	if err != nil {
		return nil, "", nil
	}

	if storedHash != hashToken(token, s.sessionSecret) {
		return nil, "", nil
	}
	if exp <= time.Now().UTC().Format(time.RFC3339) {
		_ = s.RevokeSession(ctx, sid)
		return nil, "", nil
	}

	return &u, csrf, nil
}

func (s *SessionManager) RevokeSession(ctx context.Context, sid string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, sid)
	return err
}

func (s *SessionManager) RevokeAllUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token, secret string) string {
	h := sha256.Sum256([]byte(secret + ":" + token))
	return hex.EncodeToString(h[:])
}

func splitTwo(s, sep string) []string {
	idx := indexString(s, sep)
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+len(sep):]}
}

func indexString(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
