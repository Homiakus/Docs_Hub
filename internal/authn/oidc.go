package authn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

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
	cfg OIDCConfig
}

func NewOIDCAuthService(cfg OIDCConfig) *OIDCAuthService {
	return &OIDCAuthService{cfg: cfg}
}

func (s *OIDCAuthService) GenerateStateAndNonce() (string, string) {
	bState := make([]byte, 24)
	bNonce := make([]byte, 24)
	_, _ = rand.Read(bState)
	_, _ = rand.Read(bNonce)
	return base64.RawURLEncoding.EncodeToString(bState), base64.RawURLEncoding.EncodeToString(bNonce)
}

func (s *OIDCAuthService) ProvisionUserFromClaims(ctx context.Context, claims *OIDCClaims) (*domain.User, error) {
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
