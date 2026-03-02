package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"bookarr/internal/auth"
	"bookarr/internal/config"
	"bookarr/internal/db"
	"bookarr/internal/discord"
	"bookarr/internal/library"
	"bookarr/internal/metadata"
	"bookarr/internal/prowlarr"
	"bookarr/internal/qbit"
	"bookarr/internal/requests"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := seedAdmin(context.Background(), database, cfg); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	tokenCfg := auth.TokenConfig{
		Secret: []byte(cfg.JWTSecret),
		Expiry: cfg.JWTExpiry,
	}

	// Create clients here so both the router and the watcher share the same instances.
	prowlarrClient := prowlarr.New(cfg.ProwlarrURL, cfg.ProwlarrAPIKey)
	qbitClient := qbit.New(cfg.QBitURL, cfg.QBitUsername, cfg.QBitPassword)

	// Context cancelled on SIGINT/SIGTERM for clean goroutine shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			slog.Info("shutdown signal received")
			cancel()
		case <-ctx.Done():
		}
	}()

	metaClient := metadata.New(cfg.AudnexusURL)
	mover := library.New(cfg.WatchDir, cfg.LibraryDir, cfg.WatchTimeout)

	// lookupUsername returns the username for a user ID, falling back to the
	// raw ID if the DB lookup fails (e.g. user deleted between request and completion).
	lookupUsername := func(ctx context.Context, userID string) string {
		u, err := database.GetUserByID(ctx, userID)
		if err != nil {
			return userID
		}
		return u.Username
	}

	onComplete := func(ctx context.Context, req *db.Request, info *qbit.TorrentInfo) error {
		// 1. Resolve and persist metadata.
		book := metaClient.Resolve(ctx, req.Title, req.Author)
		metaJSON, err := book.JSON()
		if err != nil {
			return fmt.Errorf("serialise metadata: %w", err)
		}
		if err := database.UpdateRequestStatus(ctx, req.ID, db.StatusMoving, db.WithMetadata(metaJSON)); err != nil {
			return fmt.Errorf("persist metadata: %w", err)
		}
		slog.Info("metadata resolved",
			"request_id", req.ID,
			"title", book.Title,
			"author", book.Author,
			"year", book.Year,
			"narrator", book.Narrator,
			"series", book.Series,
		)

		// 2. Wait for the file to appear in the watch dir (Syncthing may delay
		// delivery after qBit reports seeding), then move to the library.
		finalPath, err := mover.Move(ctx, info.Name, book)
		if err != nil {
			return fmt.Errorf("move files: %w", err)
		}
		if err := database.UpdateRequestStatus(ctx, req.ID, db.StatusMoving, db.WithFinalPath(finalPath)); err != nil {
			slog.Warn("persist final path", "request_id", req.ID, "err", err)
		}

		// 3. Discord success notification (best-effort).
		if err := discord.NotifyComplete(ctx, cfg.DiscordWebhookURL, book,
			lookupUsername(ctx, req.UserID), finalPath); err != nil {
			slog.Warn("discord notify complete", "request_id", req.ID, "err", err)
		}
		return nil
	}

	onFail := func(ctx context.Context, req *db.Request, reason string) {
		if err := discord.NotifyFailed(ctx, cfg.DiscordWebhookURL,
			req.Title, req.Author, lookupUsername(ctx, req.UserID), reason); err != nil {
			slog.Warn("discord notify failed", "request_id", req.ID, "err", err)
		}
	}

	watcher := qbit.NewWatcher(database, qbitClient, onComplete, onFail)
	watcher.Start(ctx)

	r := buildRouter(database, cfg, tokenCfg, prowlarrClient, qbitClient)

	slog.Info("server listening", "port", cfg.Port)
	return http.ListenAndServe(":"+cfg.Port, r)
}

// buildRouter wires all routes. Auth-protected routes are added in a sub-router
// that applies the Authenticate middleware. New handler groups are mounted here
// as they are built in subsequent steps.
func buildRouter(database *db.DB, cfg *config.Config, tokenCfg auth.TokenConfig, prowlarrClient *prowlarr.Client, qbitClient *qbit.Client) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	authHandler := auth.NewHandler(database, tokenCfg)
	searchHandler := prowlarr.NewHandler(prowlarrClient)
	requestsHandler := requests.New(database, prowlarrClient, qbitClient, cfg.QBitDownloadDir)

	// Public — no JWT required.
	r.Post("/api/auth/login", authHandler.Login)

	// Wizarr / provisioning: service-token auth, no user JWT required.
	r.With(auth.RequireServiceToken(cfg.ServiceToken)).Post("/api/users", authHandler.CreateUser)

	// Protected — JWT required for all routes in this group.
	r.Group(func(r chi.Router) {
		r.Use(auth.Authenticate(tokenCfg))
		r.Get("/api/auth/me", authHandler.Me)
		r.Get("/api/search", searchHandler.Search)
		r.Post("/api/requests", requestsHandler.Submit)
		r.Get("/api/requests", requestsHandler.List)
		r.Get("/api/requests/{id}", requestsHandler.Get)
	})

	return r
}

// seedAdmin creates the initial admin account from environment variables if
// the users table is empty. This is a no-op on every subsequent startup.
func seedAdmin(ctx context.Context, database *db.DB, cfg *config.Config) error {
	n, err := database.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return nil // users already exist, nothing to do
	}

	if cfg.AdminPassword == "" {
		return fmt.Errorf("no users exist: set ADMIN_USERNAME and ADMIN_PASSWORD to create the first admin account")
	}

	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	if err := database.CreateUser(ctx, uuid.NewString(), cfg.AdminUsername, hash, "admin"); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	slog.Info("created admin account", "username", cfg.AdminUsername)
	return nil
}
