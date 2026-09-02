package authn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/homiakus/docshub-next/internal/domain"
)

type OIDCConfig struct {
	Enabled      bool   `json:"enabled"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
}

type OIDCClaims struct {
	Subject       string   `json:"sub"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Name          string   `json:"name"`
	Username      string   `json:"preferred_username"`
	Groups        []string `json:"groups"`
}

type OIDCProvider interface {
	AuthURL(state, nonce string) string
	Exchange(ctx context.Context, code string) (*OIDCClaims, error)
}

type OIDCAuthService struct {
	cfg    OIDCConfig
	random io.Reader
}

func NewOIDCAuthService(cfg OIDCConfig) *OIDCAuthService {
	return &OIDCAuthService{cfg: cfg, random: rand.Reader}
}

func (s *OIDCAuthService) GenerateStateAndNonce() (string, string, error) {
	reader := rand.Reader
	if s != nil && s.random != nil {
		reader = s.random
	}

	bState := make([]byte, 24)
	if _, err := io.ReadFull(reader, bState); err != nil {
		return "", "", fmt.Errorf("oidc: generate state: %w", err)
	}
	bNonce := make([]byte, 24)
	if _, err := io.ReadFull(reader, bNonce); err != nil {
		return "", "", fmt.Errorf("oidc: generate nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bState), base64.RawURLEncoding.EncodeToString(bNonce), nil
}

func (s *OIDCAuthService) ProvisionUserFromClaims(ctx context.Context, claims *OIDCClaims) (*domain.User, error) {
	if claims == nil {
		return nil, errors.New("oidc: missing claims")
	}
	if claims.Email == "" && claims.Username == "" {
		return nil, errors.New("oidc: invalid claims missing email/username")
	}
	username := claims.Username
	if username == "" {
		username = claims.Email
	}
	displayName := claims.Name
	if displayName == "" {
		displayName = username
	}
	return &domain.User{
		Username:    username,
		DisplayName: displayName,
		Email:       claims.Email,
		Role:        "editor",
		Active:      true,
	}, nil
}
