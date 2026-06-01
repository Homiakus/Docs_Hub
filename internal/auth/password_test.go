package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_minLength(t *testing.T) {
	_, err := HashPassword("short")
	if err == nil {
		t.Fatal("expected error for password shorter than 8 characters")
	}
}

func TestHashPasswordAndVerify(t *testing.T) {
	password := "mySecurePassword123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !VerifyPassword(hash, password) {
		t.Fatal("VerifyPassword returned false for correct password")
	}
}

func TestVerifyPassword_wrongPassword(t *testing.T) {
	password := "mySecurePassword123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if VerifyPassword(hash, "wrongPassword") {
		t.Fatal("VerifyPassword returned true for wrong password")
	}
}

func TestVerifyPassword_tamperedHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"invalid format", "invalid$hash$format"},
		{"missing parts", "argon2id$v=19$m=65536,t=3,p=2$salt"},
		{"bad params", "argon2id$v=19$m=bad,t=bad,p=bad$salt$hash"},
		{"invalid base64 salt", "argon2id$v=19$m=65536,t=3,p=2$!!!invalid!!!$hash"},
		{"wrong prefix", "bcrypt$$2a$10$abcdefghijklmnopqrstuv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if VerifyPassword(tt.hash, "password123") {
				t.Fatalf("VerifyPassword returned true for hash: %s", tt.name)
			}
		})
	}
}

func TestHashPassword_uniqueness(t *testing.T) {
	password := "mySecurePassword123"
	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash1 == hash2 {
		t.Fatal("two hashes of the same password should differ due to random salt")
	}
}

func TestHashPassword_format(t *testing.T) {
	hash, err := HashPassword("validPassword123")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !strings.HasPrefix(hash, "argon2id$") {
		t.Fatalf("hash should start with argon2id$, got: %s", hash)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 5 {
		t.Fatalf("hash should have 5 parts, got %d: %s", len(parts), hash)
	}
}

func TestVerifyPassword_emptyHash(t *testing.T) {
	if VerifyPassword("", "password123") {
		t.Fatal("VerifyPassword returned true for empty hash")
	}
}

func TestHashPassword_exactlyEightChars(t *testing.T) {
	_, err := HashPassword("12345678")
	if err != nil {
		t.Fatalf("HashPassword should accept 8-char password: %v", err)
	}
}
