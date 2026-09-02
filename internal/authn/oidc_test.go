package authn

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

type oidcFailAfterStateReader struct {
	servedState bool
}

func (r *oidcFailAfterStateReader) Read(p []byte) (int, error) {
	if r.servedState {
		return 0, errors.New("entropy unavailable")
	}
	r.servedState = true
	if len(p) < 24 {
		return 0, errors.New("unexpected short state buffer")
	}
	for i := 0; i < 24; i++ {
		p[i] = 0xA5
	}
	return 24, nil
}

func TestOIDCGenerateStateAndNonceUsesFullEntropy(t *testing.T) {
	stateBytes := bytes.Repeat([]byte{0x11}, 24)
	nonceBytes := bytes.Repeat([]byte{0x22}, 24)
	stream := append(append([]byte{}, stateBytes...), nonceBytes...)

	svc := NewOIDCAuthService(OIDCConfig{})
	svc.random = bytes.NewReader(stream)
	state, nonce, err := svc.GenerateStateAndNonce()
	if err != nil {
		t.Fatalf("GenerateStateAndNonce: %v", err)
	}
	if want := base64.RawURLEncoding.EncodeToString(stateBytes); state != want {
		t.Fatalf("state=%q want %q", state, want)
	}
	if want := base64.RawURLEncoding.EncodeToString(nonceBytes); nonce != want {
		t.Fatalf("nonce=%q want %q", nonce, want)
	}
	if state == nonce {
		t.Fatal("state and nonce must be independently generated")
	}
}

func TestOIDCGenerateStateAndNonceFailsClosedOnEntropyError(t *testing.T) {
	svc := NewOIDCAuthService(OIDCConfig{})
	svc.random = &oidcFailAfterStateReader{}

	state, nonce, err := svc.GenerateStateAndNonce()
	if err == nil {
		t.Fatal("expected nonce entropy failure")
	}
	if state != "" || nonce != "" {
		t.Fatalf("entropy failure exposed partial auth material: state=%q nonce=%q", state, nonce)
	}
}

func TestOIDCProvisionUserRejectsNilClaims(t *testing.T) {
	svc := NewOIDCAuthService(OIDCConfig{})
	if _, err := svc.ProvisionUserFromClaims(t.Context(), nil); err == nil {
		t.Fatal("nil claims must be rejected")
	}
}
