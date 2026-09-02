package authn

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
)

const sessionTouchInterval = 5 * time.Minute

type SessionManager struct {
	db            *db.DB
	sessionSecret string
	idleTimeout   time.Duration
	maxLifetime   time.Duration
	random        io.Reader
}

func NewSessionManager(d *db.DB, secret string) *SessionManager {
	return &SessionManager{
		db:            d,
		sessionSecret: secret,
		idleTimeout:   24 * time.Hour,
		maxLifetime:   7 * 24 * time.Hour,
		random:        rand.Reader,
	}
}

func (s *SessionManager) CreateSession(ctx context.Context, userID int64, ip, userAgent string) (string, string, string, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return "", "", "", errors.New("session: invalid manager or user")
	}

	sid, err := s.randomString(24)
	if err != nil {
		return "", "", "", fmt.Errorf("generate session id: %w", err)
	}
	token, err := s.randomString(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate session token: %w", err)
	}
	csrf, err := s.randomString(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate csrf token: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.maxLifetime).Format(time.RFC3339)
	createdAt := now.Format(time.RFC3339)
	tokenHash := hashToken(token, s.sessionSecret)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions(
			id, token_hash, user_id, csrf_token, expires_at, created_at,
			last_seen_at, client_ip, user_agent
		) VALUES(?,?,?,?,?,?,?,?,?)
	`, sid, tokenHash, userID, csrf, expiresAt, createdAt, createdAt, boundedSessionText(ip, 255), boundedSessionText(userAgent, 1024))
	if err != nil {
		return "", "", "", err
	}

	cookieValue := sid + "." + token
	return cookieValue, csrf, expiresAt, nil
}

func (s *SessionManager) ValidateSession(ctx context.Context, cookieValue string) (*domain.User, string, error) {
	if s == nil || s.db == nil || cookieValue == "" {
		return nil, "", nil
	}
	parts := splitTwo(cookieValue, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, "", nil
	}
	sid, token := parts[0], parts[1]

	var u domain.User
	var storedHash, expRaw, lastSeenRaw, csrf string
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.display_name, coalesce(u.email,''), u.role, u.is_active,
		       s.token_hash, s.expires_at, coalesce(s.last_seen_at, s.created_at), s.csrf_token
		FROM sessions s
		JOIN users u ON u.id=s.user_id
		WHERE s.id=? AND u.is_active=1
	`, sid).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Active, &storedHash, &expRaw, &lastSeenRaw, &csrf)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}

	if !tokenHashMatches(storedHash, token, s.sessionSecret) {
		return nil, "", nil
	}

	expiresAt, err := time.Parse(time.RFC3339, expRaw)
	if err != nil {
		_ = s.RevokeSession(ctx, sid)
		return nil, "", fmt.Errorf("session: invalid expires_at: %w", err)
	}
	lastSeenAt, err := time.Parse(time.RFC3339, lastSeenRaw)
	if err != nil {
		_ = s.RevokeSession(ctx, sid)
		return nil, "", fmt.Errorf("session: invalid last_seen_at: %w", err)
	}

	now := time.Now().UTC()
	if !now.Before(expiresAt) || (s.idleTimeout > 0 && now.Sub(lastSeenAt) > s.idleTimeout) {
		_ = s.RevokeSession(ctx, sid)
		return nil, "", nil
	}

	if now.Sub(lastSeenAt) >= sessionTouchInterval {
		result, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE id=?`, now.Format(time.RFC3339), sid)
		if err != nil {
			return nil, "", err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			if err != nil {
				return nil, "", err
			}
			return nil, "", errors.New("session: touch affected unexpected row count")
		}
	}

	return &u, csrf, nil
}

func (s *SessionManager) RevokeSession(ctx context.Context, sid string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, sid)
	return err
}

func (s *SessionManager) RevokeAllUserSessions(ctx context.Context, userID int64) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

func (s *SessionManager) randomString(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("session: random length must be positive")
	}
	reader := s.random
	if reader == nil {
		reader = rand.Reader
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenMAC(token, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return mac.Sum(nil)
}

func hashToken(token, secret string) string {
	return hex.EncodeToString(tokenMAC(token, secret))
}

func tokenHashMatches(storedHash, token, secret string) bool {
	storedMAC, err := hex.DecodeString(storedHash)
	if err != nil {
		return false
	}
	return hmac.Equal(storedMAC, tokenMAC(token, secret))
}

func boundedSessionText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
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
