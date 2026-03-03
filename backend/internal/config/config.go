// Package config loads all application configuration from environment variables.
// Only JWT_SECRET is required at startup; service credentials (Prowlarr, qBit,
// etc.) will fail at the point of use when those features are exercised.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all application configuration read from environment variables.
type Config struct {
	// Database
	DBPath string

	// Prowlarr
	ProwlarrURL    string
	ProwlarrAPIKey string

	// qBittorrent
	QBitURL      string
	QBitUsername string
	QBitPassword string
	QBitCategory string
	QBitAutoTMM  bool

	// Library
	// WatchDir is the local path where completed files appear (either directly
	// from qBit or delivered by Syncthing for remote seedbox setups). If empty,
	// main resolves it from qBittorrent's configured save path at startup.
	WatchDir   string
	LibraryDir string
	// WatchTimeout is the maximum time Move will poll WatchDir before giving up.
	WatchTimeout time.Duration

	// Audiobookshelf
	ABSURL string

	// Discord
	DiscordWebhookURL string

	// Auth
	JWTSecret string        `json:"-"` //nolint:gosec
	JWTExpiry time.Duration `json:"-"`

	// Server
	Port      string
	StaticDir string // directory to serve the frontend SPA from
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

	jwtExpiry, err := time.ParseDuration(getenv("JWT_EXPIRY", "24h"))
	if err != nil {
		return nil, fmt.Errorf("parse JWT_EXPIRY: %w", err)
	}

	watchTimeout, err := time.ParseDuration(getenv("WATCH_TIMEOUT", "24h"))
	if err != nil {
		return nil, fmt.Errorf("parse WATCH_TIMEOUT: %w", err)
	}

	return &Config{
		DBPath:            getenv("DB_PATH", "/data/shelfarr.db"),
		ProwlarrURL:       getenv("PROWLARR_URL", ""),
		ProwlarrAPIKey:    getenv("PROWLARR_API_KEY", ""),
		QBitURL:           getenv("QBIT_URL", ""),
		QBitUsername:      getenv("QBIT_USERNAME", "admin"),
		QBitPassword:      getenv("QBIT_PASSWORD", ""),
		QBitCategory:      getenv("QBIT_CATEGORY", ""),
		QBitAutoTMM:       os.Getenv("QBIT_AUTO_TMM") == "true",
		WatchDir:          getenv("WATCH_DIR", ""),
		LibraryDir:        getenv("LIBRARY_DIR", "/audiobooks"),
		WatchTimeout:      watchTimeout,
		ABSURL:            absURL,
		DiscordWebhookURL: getenv("DISCORD_WEBHOOK_URL", ""),
		JWTSecret:         jwtSecret,
		JWTExpiry:         jwtExpiry,
		Port:              getenv("PORT", "8008"),
		StaticDir:         getenv("STATIC_DIR", "/app/static"),
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
