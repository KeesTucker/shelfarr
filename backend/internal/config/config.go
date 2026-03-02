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
	DownloadDir  string
	QBitCategory string

	// Metadata sources
	AudnexusURL string // optional; empty string disables Audnexus lookups

	// Library
	// WatchDir is the local path where Syncthing delivers completed files from
	// the seedbox. Defaults to DOWNLOAD_DIR for non-Syncthing setups where
	// qBit runs locally and files are accessible directly.
	WatchDir   string
	LibraryDir string
	// WatchTimeout is the maximum time Move will poll WatchDir before giving up.
	WatchTimeout time.Duration

	// Audiobookshelf
	ABSURL       string
	ABSAPIKey    string
	ABSLibraryID string

	// Discord
	DiscordWebhookURL string

	// Auth
	JWTSecret string
	JWTExpiry time.Duration

	// Admin seed — only needed on first startup when no users exist.
	AdminUsername string
	AdminPassword string

	// Wizarr integration
	ServiceToken string

	// Server
	Port string
}

// Load reads configuration from the environment. Returns an error if any
// required variable is absent or malformed.
func Load() (*Config, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET must be set")
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
		DBPath:            getenv("DB_PATH", "/data/bookarr.db"),
		ProwlarrURL:       getenv("PROWLARR_URL", ""),
		ProwlarrAPIKey:    getenv("PROWLARR_API_KEY", ""),
		QBitURL:           getenv("QBIT_URL", ""),
		QBitUsername:      getenv("QBIT_USERNAME", "admin"),
		QBitPassword:      getenv("QBIT_PASSWORD", ""),
		DownloadDir:       getenv("DOWNLOAD_DIR", "/downloads/audiobooks"),
		QBitCategory:      getenv("QBIT_CATEGORY", ""),
		AudnexusURL:       getenv("AUDNEXUS_URL", ""),
		WatchDir:          getenv("WATCH_DIR", getenv("DOWNLOAD_DIR", "/downloads/audiobooks")),
		LibraryDir:        getenv("LIBRARY_DIR", "/audiobooks"),
		WatchTimeout:      watchTimeout,
		ABSURL:            getenv("ABS_URL", ""),
		ABSAPIKey:         getenv("ABS_API_KEY", ""),
		ABSLibraryID:      getenv("ABS_LIBRARY_ID", ""),
		DiscordWebhookURL: getenv("DISCORD_WEBHOOK_URL", ""),
		JWTSecret:         jwtSecret,
		JWTExpiry:         jwtExpiry,
		AdminUsername:     getenv("ADMIN_USERNAME", "admin"),
		AdminPassword:     getenv("ADMIN_PASSWORD", ""),
		ServiceToken:      getenv("SERVICE_TOKEN", ""),
		Port:              getenv("PORT", "8080"),
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
