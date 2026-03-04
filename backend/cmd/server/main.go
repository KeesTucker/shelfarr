package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"shelfarr/internal/abs"
	"shelfarr/internal/auth"
	"shelfarr/internal/config"
	"shelfarr/internal/db"
	"shelfarr/internal/discord"
	"shelfarr/internal/library"
	"shelfarr/internal/metadata"
	"shelfarr/internal/prowlarr"
	"shelfarr/internal/qbit"
	"shelfarr/internal/requests"
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
	defer func() {
		if err := database.Close(); err != nil {
			slog.Error("close database", "err", err)
		}
	}()

	tokenCfg := auth.TokenConfig{
		Secret:       []byte(cfg.JWTSecret),
		Expiry:       24 * time.Hour,
		CookieSecure: cfg.CookieSecure,
	}

	// Create clients here so both the router and the watcher share the same instances.
	absClient := abs.New(cfg.ABSURL)
	prowlarrClient := prowlarr.New(cfg.ProwlarrURL, cfg.ProwlarrAPIKey)
	qbitClient := qbit.New(cfg.QBitURL, cfg.QBitUsername, cfg.QBitPassword)
	qbitClient.SetAutoTMM(cfg.QBitAutoTMM)

	// ctx is cancelled on SIGINT/SIGTERM to stop background goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	metaClient := metadata.New()

	mover := library.New(cfg.WatchDir, cfg.LibraryDir, cfg.WatchTimeout)
	libraryHandler := library.NewHandler(cfg.LibraryDir)

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
		// 1. Resolve metadata — use user-selected metadata if available, otherwise
		// fall back to an automatic OpenLibrary lookup.
		var book *metadata.Book
		if req.MetadataJSON.Valid && req.MetadataJSON.String != "" {
			b, err := metadata.FromJSON(req.MetadataJSON.String)
			if err != nil {
				slog.Warn("metadata: bad stored JSON, re-fetching", "request_id", req.ID, "err", err)
				book = metaClient.Resolve(ctx, req.Title, req.Author)
			} else {
				book = b
			}
		} else {
			book = metaClient.Resolve(ctx, req.Title, req.Author)
		}

		metaJSON, err := book.JSON()
		if err != nil {
			return fmt.Errorf("serialise metadata: %w", err)
		}
		if err := database.UpdateRequestStatus(ctx, req.ID, db.StatusImporting, db.WithMetadata(metaJSON)); err != nil {
			return fmt.Errorf("persist metadata: %w", err)
		}
		slog.Info("metadata resolved",
			"request_id", req.ID,
			"title", book.Title,
			"author", book.Author,
			"year", book.Year,
		)

		// 2. Wait for the file to appear in the watch dir (Syncthing may delay
		// delivery after qBit reports seeding), then move to the library.
		finalPath, err := mover.Move(ctx, info.Name, book)
		if err != nil {
			return fmt.Errorf("move files: %w", err)
		}
		if err := database.UpdateRequestStatus(ctx, req.ID, db.StatusImporting, db.WithFinalPath(finalPath)); err != nil {
			slog.Warn("persist final path", "request_id", req.ID, "err", err)
		}

		// 3. Write OPF sidecar if none already present (best-effort).
		if err := metadata.EnsureOPF(finalPath, book); err != nil {
			slog.Warn("metadata: write OPF", "request_id", req.ID, "path", finalPath, "err", err)
		}

		// 4. Set the "imported" category in qBittorrent (best-effort).
		if cfg.QBitImportedCategory != "" && info.Hash != "" {
			if err := qbitClient.SetCategory(ctx, info.Hash, cfg.QBitImportedCategory); err != nil {
				slog.Warn("qbit: set imported category", "request_id", req.ID, "hash", info.Hash, "err", err)
			}
		}

		// 5. Discord success notification (best-effort).
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

	// Reset requests that were left in "importing" status from a previous run.
	// Their goroutines died with the process and cannot be resumed.
	if n, err := database.FailStuckImportingRequests(ctx); err != nil {
		slog.Warn("reset stuck importing requests", "err", err)
	} else if n > 0 {
		slog.Info("reset stuck importing requests to failed", "count", n)
	}

	watcher := qbit.NewWatcher(database, qbitClient, onComplete, onFail)
	watcher.Start(ctx)

	// onImport adapts onComplete for files already in the watch dir: there is
	// no real TorrentInfo, so we synthesise one using the entry name.
	onImportFn := func(ctx context.Context, req *db.Request, torrentName string) error {
		return onComplete(ctx, req, &qbit.TorrentInfo{Name: torrentName})
	}

	requestsHandler := requests.New(database, prowlarrClient, qbitClient, cfg.QBitCategory)
	requestsHandler.SetDeleteTorrentsOnRequestDelete(cfg.QBitDeleteOnRequestDelete)
	requestsHandler.SetImportConfig(ctx, cfg.WatchDir, onImportFn, onFail)
	metaHandler := metadata.NewHandler(metaClient)

	r := buildRouter(database, tokenCfg, absClient, prowlarrClient, requestsHandler, metaHandler, libraryHandler, cfg.StaticDir)

	const port = "8008"
	slog.Info("server listening", "port", port)
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	srvErr := make(chan error, 1)
	go func() { srvErr <- srv.ListenAndServe() }()

	select {
	case err := <-srvErr:
		// Server stopped on its own (e.g. bind error) — not a clean shutdown.
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-sigCh:
		slog.Info("shutdown signal received")
	}

	// Cancel background goroutines (watcher, import pipeline).
	cancel()

	// Give in-flight HTTP requests up to 10 s to complete.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown", "err", err)
	}
	return nil
}

// buildRouter wires all routes. Auth-protected routes are added in a sub-router
// that applies the Authenticate middleware.
func buildRouter(database *db.DB, tokenCfg auth.TokenConfig, absClient *abs.Client, prowlarrClient *prowlarr.Client, requestsHandler *requests.Handler, metaHandler *metadata.Handler, libraryHandler *library.Handler, staticDir string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	authHandler := auth.NewHandler(database, tokenCfg, absClient)
	searchHandler := prowlarr.NewHandler(prowlarrClient)

	// Public — no JWT required.
	r.Post("/api/auth/login", authHandler.Login)
	r.Post("/api/auth/logout", authHandler.Logout)

	// Protected — JWT required for all routes in this group.
	r.Group(func(r chi.Router) {
		r.Use(auth.Authenticate(tokenCfg))
		r.Get("/api/auth/me", authHandler.Me)
		r.Get("/api/search", searchHandler.Search)
		r.Get("/api/metadata/search", metaHandler.Search)
		r.Post("/api/requests", requestsHandler.Submit)
		r.Get("/api/requests", requestsHandler.List)
		r.Get("/api/requests/{id}", requestsHandler.Get)
		r.Delete("/api/requests/{id}", requestsHandler.Delete)

		// Admin-only routes.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAdmin)
			r.Get("/api/watchdir", requestsHandler.ListWatchDir)
			r.Post("/api/import", requestsHandler.Import)
			r.Get("/api/library", libraryHandler.List)
			r.Post("/api/library/cleanup", libraryHandler.Cleanup)
			r.Post("/api/library/prune", libraryHandler.Prune)
		})
	})

	// Serve the frontend SPA for all non-API paths.
	r.Handle("/*", spaHandler(staticDir))

	return r
}
