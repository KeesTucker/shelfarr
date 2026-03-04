// Package config loads all application configuration from environment variables.
// Only JWT_SECRET and ABS_URL are required at startup; service credentials
// (Prowlarr, qBit, etc.) will fail at the point of use when those features
// are exercised.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all application configuration read from environment variables.
type Config struct {
	// Database
	DBPath string // DB_PATH (default: /data/shelfarr.db)

	// Prowlarr
	ProwlarrURL    string
	ProwlarrAPIKey string

	// qBittorrent
	QBitURL                   string
	QBitUsername              string
	QBitPassword              string
	QBitCategory              string
	QBitImportedCategory      string // QBIT_IMPORTED_CATEGORY — set after file is moved to library
	QBitAutoTMM               bool
	QBitDeleteOnRequestDelete bool // QBIT_DELETE_ON_REQUEST_DELETE

	// Library
	WatchDir     string        // WATCH_DIR (default: /downloads)
	LibraryDir   string        // LIBRARY_DIR (default: /audiobooks)
	WatchTimeout time.Duration // WATCH_TIMEOUT (default: 24h)

	// Audiobookshelf
	ABSURL string

	// Discord
	DiscordWebhookURL string

	// Auth
	JWTSecret    string `json:"-"` //nolint:gosec
	CookieSecure bool   // set false for local dev over plain HTTP

	// Server
	StaticDir string // STATIC_DIR (default: /app/static)
}

// Load reads configuration from the environment. Returns an error if any
// required variable is absent or malformed.
func Load() (*Config, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET must be set")
	}

	absURL := os.Getenv("ABS_URL")
	if absURL == "" {
		return nil, fmt.Errorf("ABS_URL must be set (e.g. http://audiobookshelf:13378)")
	}

	watchTimeout, err := time.ParseDuration(getenv("WATCH_TIMEOUT", "24h"))
	if err != nil {
		return nil, fmt.Errorf("parse WATCH_TIMEOUT: %w", err)
	}

	return &Config{
		DBPath:                    getenv("DB_PATH", "/data/shelfarr.db"),
		ProwlarrURL:               getenv("PROWLARR_URL", ""),
		ProwlarrAPIKey:            getenv("PROWLARR_API_KEY", ""),
		QBitURL:                   getenv("QBIT_URL", ""),
		QBitUsername:              getenv("QBIT_USERNAME", "admin"),
		QBitPassword:              getenv("QBIT_PASSWORD", ""),
		QBitCategory:              getenv("QBIT_CATEGORY", ""),
		QBitImportedCategory:      getenv("QBIT_IMPORTED_CATEGORY", ""),
		QBitAutoTMM:               os.Getenv("QBIT_AUTO_TMM") == "true",
		QBitDeleteOnRequestDelete: os.Getenv("QBIT_DELETE_ON_REQUEST_DELETE") == "true",
		WatchDir:                  getenv("WATCH_DIR", "/downloads"),
		LibraryDir:                getenv("LIBRARY_DIR", "/audiobooks"),
		WatchTimeout:              watchTimeout,
		ABSURL:                    absURL,
		DiscordWebhookURL:         getenv("DISCORD_WEBHOOK_URL", ""),
		JWTSecret:                 jwtSecret,
		CookieSecure:              os.Getenv("COOKIE_INSECURE") != "true",
		StaticDir:                 getenv("STATIC_DIR", "/app/static"),
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
