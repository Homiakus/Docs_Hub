package config

import (
	"os"
	"testing"
)

func TestLoad_defaults(t *testing.T) {
	// Clear all relevant env vars to test defaults
	envVars := []string{
		"DATA_DIR", "ADDR", "DB_PATH", "UPLOAD_DIR", "SITE_NAME",
		"ADMIN_USER", "ADMIN_PASSWORD", "SESSION_SECRET", "COOKIE_SECURE",
		"TLS_ENABLED", "TLS_CERT_FILE", "TLS_KEY_FILE", "LOG_LEVEL",
		"RATE_LIMIT_ENABLED", "RATE_LIMIT_RPM", "RATE_LIMIT_BURST",
	}
	saved := make(map[string]string)
	for _, k := range envVars {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	cfg := Load()

	if cfg.Addr != ":8080" {
		t.Errorf("expected Addr :8080, got %s", cfg.Addr)
	}
	if cfg.SiteName != "Docs Hub Next" {
		t.Errorf("expected SiteName 'Docs Hub Next', got %q", cfg.SiteName)
	}
	if cfg.AdminUser != "admin" {
		t.Errorf("expected AdminUser 'admin', got %s", cfg.AdminUser)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel 'info', got %s", cfg.LogLevel)
	}
	if cfg.RateLimit.Enabled != true {
		t.Error("expected RateLimit.Enabled true")
	}
	if cfg.RateLimit.RequestsPerMin != 60 {
		t.Errorf("expected RateLimit.RequestsPerMin 60, got %d", cfg.RateLimit.RequestsPerMin)
	}
	if cfg.RateLimit.Burst != 10 {
		t.Errorf("expected RateLimit.Burst 10, got %d", cfg.RateLimit.Burst)
	}
	if cfg.CookieSecure != false {
		t.Error("expected CookieSecure false")
	}
	if cfg.TLS.Enabled != false {
		t.Error("expected TLS.Enabled false")
	}
	if cfg.AdminPassword != "" {
		t.Errorf("expected AdminPassword empty by default, got %q", cfg.AdminPassword)
	}
	if cfg.SessionSecret != "" {
		t.Errorf("expected SessionSecret empty by default, got %q", cfg.SessionSecret)
	}
}

func TestLoad_customValues(t *testing.T) {
	envVars := map[string]string{
		"ADDR":               ":9090",
		"SITE_NAME":          "Test Site",
		"ADMIN_USER":         "root",
		"ADMIN_PASSWORD":     "secret123",
		"SESSION_SECRET":     "session-key-32-bytes-long!!",
		"COOKIE_SECURE":      "true",
		"LOG_LEVEL":          "debug",
		"RATE_LIMIT_ENABLED": "false",
		"RATE_LIMIT_RPM":     "120",
		"RATE_LIMIT_BURST":   "20",
		"TLS_ENABLED":        "1",
		"TLS_CERT_FILE":      "/tmp/cert.pem",
		"TLS_KEY_FILE":       "/tmp/key.pem",
		"DATA_DIR":           "/custom/data",
	}
	saved := make(map[string]string)
	for k := range envVars {
		saved[k] = os.Getenv(k)
		os.Setenv(k, envVars[k])
	}
	defer func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	cfg := Load()

	if cfg.Addr != ":9090" {
		t.Errorf("expected Addr :9090, got %s", cfg.Addr)
	}
	if cfg.SiteName != "Test Site" {
		t.Errorf("expected SiteName 'Test Site', got %q", cfg.SiteName)
	}
	if cfg.AdminUser != "root" {
		t.Errorf("expected AdminUser 'root', got %s", cfg.AdminUser)
	}
	if cfg.AdminPassword != "secret123" {
		t.Errorf("expected AdminPassword 'secret123', got %q", cfg.AdminPassword)
	}
	if cfg.SessionSecret != "session-key-32-bytes-long!!" {
		t.Errorf("expected SessionSecret set, got %q", cfg.SessionSecret)
	}
	if cfg.CookieSecure != true {
		t.Error("expected CookieSecure true")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel 'debug', got %s", cfg.LogLevel)
	}
	if cfg.RateLimit.Enabled != false {
		t.Error("expected RateLimit.Enabled false")
	}
	if cfg.RateLimit.RequestsPerMin != 120 {
		t.Errorf("expected RateLimit.RequestsPerMin 120, got %d", cfg.RateLimit.RequestsPerMin)
	}
	if cfg.RateLimit.Burst != 20 {
		t.Errorf("expected RateLimit.Burst 20, got %d", cfg.RateLimit.Burst)
	}
	if cfg.TLS.Enabled != true {
		t.Error("expected TLS.Enabled true with TLS_ENABLED=1")
	}
	if cfg.DataDir != "/custom/data" {
		t.Errorf("expected DataDir '/custom/data', got %q", cfg.DataDir)
	}
}

func TestLoad_cookieSecureTrue(t *testing.T) {
	os.Setenv("COOKIE_SECURE", "1")
	defer os.Unsetenv("COOKIE_SECURE")
	cfg := Load()
	if cfg.CookieSecure != true {
		t.Error("expected CookieSecure true with COOKIE_SECURE=1")
	}
}

func TestLoad_rateLimitEnabledViaZero(t *testing.T) {
	os.Setenv("RATE_LIMIT_ENABLED", "0")
	defer os.Unsetenv("RATE_LIMIT_ENABLED")
	cfg := Load()
	if cfg.RateLimit.Enabled != false {
		t.Error("expected RateLimit.Enabled false with RATE_LIMIT_ENABLED=0")
	}
}

func TestValidate_missingAdminPassword(t *testing.T) {
	cfg := Config{
		AdminPassword: "",
		SessionSecret: "super-secret-session-key-that-is-long-enough",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing AdminPassword")
	}
}

func TestValidate_missingSessionSecret(t *testing.T) {
	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing SessionSecret")
	}
}

func TestValidate_shortSessionSecret(t *testing.T) {
	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "short",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for short SessionSecret")
	}
}

func TestValidate_shortAdminPassword(t *testing.T) {
	cfg := Config{
		AdminPassword: "short",
		SessionSecret: "super-secret-session-key-that-is-long-enough",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for short AdminPassword")
	}
}

func TestValidate_TLS_missingCert(t *testing.T) {
	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "super-secret-session-key-that-is-long-enough",
		TLS: TLSConfig{
			Enabled:  true,
			CertFile: "",
			KeyFile:  "/tmp/key.pem",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing TLS cert file")
	}
}

func TestValidate_TLS_missingKey(t *testing.T) {
	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "super-secret-session-key-that-is-long-enough",
		TLS: TLSConfig{
			Enabled:  true,
			CertFile: "/tmp/cert.pem",
			KeyFile:  "",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing TLS key file")
	}
}

func TestValidate_success(t *testing.T) {
	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "super-secret-session-key-that-is-long-enough",
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_TLS_fileNotFound(t *testing.T) {
	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "super-secret-session-key-that-is-long-enough",
		TLS: TLSConfig{
			Enabled:  true,
			CertFile: "/nonexistent/cert.pem",
			KeyFile:  "/nonexistent/key.pem",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for nonexistent TLS cert file")
	}
}

func TestValidate_TLS_keyNotFound(t *testing.T) {
	// Create a temp cert file but point to nonexistent key
	tmpFile, err := os.CreateTemp("", "test-cert-*.pem")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "super-secret-session-key-that-is-long-enough",
		TLS: TLSConfig{
			Enabled:  true,
			CertFile: tmpFile.Name(),
			KeyFile:  "/nonexistent/key.pem",
		},
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("expected error for nonexistent TLS key file when cert exists")
	}
}

func TestIntEnv(t *testing.T) {
	os.Setenv("TEST_INT_VALID", "42")
	os.Setenv("TEST_INT_INVALID", "notanumber")
	os.Setenv("TEST_INT_ZERO", "0")
	os.Setenv("TEST_INT_NEGATIVE", "-5")
	defer func() {
		for _, k := range []string{"TEST_INT_VALID", "TEST_INT_INVALID", "TEST_INT_ZERO", "TEST_INT_NEGATIVE"} {
			os.Unsetenv(k)
		}
	}()

	if v := intEnv("TEST_INT_VALID", 100); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	if v := intEnv("TEST_INT_INVALID", 100); v != 100 {
		t.Errorf("expected fallback 100 for invalid, got %d", v)
	}
	if v := intEnv("TEST_INT_ZERO", 100); v != 100 {
		t.Errorf("expected fallback 100 for zero (n>0 check), got %d", v)
	}
	if v := intEnv("TEST_INT_NEGATIVE", 100); v != 100 {
		t.Errorf("expected fallback 100 for negative (n>0 check), got %d", v)
	}
	if v := intEnv("TEST_INT_MISSING", 100); v != 100 {
		t.Errorf("expected fallback 100 for missing, got %d", v)
	}
}

func TestGetenv(t *testing.T) {
	os.Setenv("TEST_GETENV_SET", "custom_value")
	defer os.Unsetenv("TEST_GETENV_SET")

	if v := getenv("TEST_GETENV_SET", "fallback"); v != "custom_value" {
		t.Errorf("expected 'custom_value', got %s", v)
	}
	if v := getenv("TEST_GETENV_MISSING", "fallback"); v != "fallback" {
		t.Errorf("expected 'fallback', got %s", v)
	}
}

func TestValidate_exactly16CharSecret(t *testing.T) {
	cfg := Config{
		AdminPassword: "validPassword123",
		SessionSecret: "1234567890123456", // exactly 16 chars
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error for exactly 16-char secret, got: %v", err)
	}
}
