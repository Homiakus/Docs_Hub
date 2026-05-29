package config

import (
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
}

func Load() Config {
	dataDir := getenv("DATA_DIR", "./data")
	return Config{
		Addr:          getenv("ADDR", ":8080"),
		DataDir:       dataDir,
		DBPath:        getenv("DB_PATH", filepath.Join(dataDir, "docshub.db")),
		UploadDir:     getenv("UPLOAD_DIR", filepath.Join(dataDir, "uploads")),
		SiteName:      getenv("SITE_NAME", "Docs Hub Next"),
		AdminUser:     getenv("ADMIN_USER", "admin"),
		AdminPassword: getenv("ADMIN_PASSWORD", "admin123"),
		SessionSecret: getenv("SESSION_SECRET", "dev-secret-change-me"),
		CookieSecure:  os.Getenv("COOKIE_SECURE") == "1" || os.Getenv("COOKIE_SECURE") == "true",
	}
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
