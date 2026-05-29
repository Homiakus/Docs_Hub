package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Addr          string
	DataDir       string
	DBPath        string
	UploadDir     string
	SiteName      string
	AdminUser     string
	AdminPassword string
	SessionSecret string
	CookieSecure  bool
	TLS           TLSConfig
	LogLevel      string
	RateLimit     RateLimitConfig
}

type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

type RateLimitConfig struct {
	Enabled      bool
	RequestsPerMin int
	Burst        int
}

func Load() Config {
	dataDir := getenv("DATA_DIR", "./data")
	cfg := Config{
		Addr:          getenv("ADDR", ":8080"),
		DataDir:       dataDir,
		DBPath:        getenv("DB_PATH", filepath.Join(dataDir, "docshub.db")),
		UploadDir:     getenv("UPLOAD_DIR", filepath.Join(dataDir, "uploads")),
		SiteName:      getenv("SITE_NAME", "Docs Hub Next"),
		AdminUser:     getenv("ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		SessionSecret: os.Getenv("SESSION_SECRET"),
		CookieSecure:  os.Getenv("COOKIE_SECURE") == "1" || os.Getenv("COOKIE_SECURE") == "true",
		TLS: TLSConfig{
			Enabled:  os.Getenv("TLS_ENABLED") == "1" || os.Getenv("TLS_ENABLED") == "true",
			CertFile: getenv("TLS_CERT_FILE", ""),
			KeyFile:  getenv("TLS_KEY_FILE", ""),
		},
		LogLevel: getenv("LOG_LEVEL", "info"),
		RateLimit: RateLimitConfig{
			Enabled:        getenv("RATE_LIMIT_ENABLED", "true") != "0" && getenv("RATE_LIMIT_ENABLED", "true") != "false",
			RequestsPerMin: intEnv("RATE_LIMIT_RPM", 60),
			Burst:         intEnv("RATE_LIMIT_BURST", 10),
		},
	}
	return cfg
}

func (c Config) Validate() error {
	if c.AdminPassword == "" {
		return errors.New("ADMIN_PASSWORD is required — set it via environment variable")
	}
	if c.SessionSecret == "" {
		return errors.New("SESSION_SECRET is required — set it via environment variable (use at least 32 random bytes)")
	}
	if len(c.SessionSecret) < 16 {
		return errors.New("SESSION_SECRET must be at least 16 characters")
	}
	if len(c.AdminPassword) < 8 {
		return errors.New("ADMIN_PASSWORD must be at least 8 characters")
	}
	if c.TLS.Enabled {
		if c.TLS.CertFile == "" {
			return errors.New("TLS_CERT_FILE is required when TLS is enabled")
		}
		if c.TLS.KeyFile == "" {
			return errors.New("TLS_KEY_FILE is required when TLS is enabled")
		}
		if _, err := os.Stat(c.TLS.CertFile); err != nil {
			return fmt.Errorf("TLS cert file not found: %s — %w", c.TLS.CertFile, err)
		}
		if _, err := os.Stat(c.TLS.KeyFile); err != nil {
			return fmt.Errorf("TLS key file not found: %s — %w", c.TLS.KeyFile, err)
		}
	}
	return nil
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func intEnv(k string, fallback int) int {
	if v := os.Getenv(k); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
