// Command econome is the HTTP server entrypoint — the binary shipped in the
// container image and built locally as econome.exe.
//
// At increment 0 it loads configuration, starts the (stub) server, logs
// structured output to stdout, and shuts down gracefully on SIGINT/SIGTERM.
// Migrations, auth, the middleware chain, and the screens are added by later
// increments.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"econome/internal/config"
	"econome/internal/i18n"
	"econome/internal/repo"
	"econome/internal/server"
	"econome/internal/services"
	"econome/internal/view"
	"econome/migrations"
)

// version is overridden at build time via -ldflags "-X main.version=vX.Y.Z"
// (guardrails/04 §4); the binary reports the released version.
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("econome: fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	initLogging(cfg.LogLevel)
	slog.Info("econome starting", "version", version, "listen", cfg.Listen, "behind_tls", cfg.BehindTLS)

	// Data volume must be writable; the session secret lives on it (technical/07 §4).
	if err := cfg.EnsureDataDir(); err != nil {
		return err
	}
	secret, err := cfg.EnsureSecret()
	if err != nil {
		return err
	}

	// Open the database and apply pending migrations (with a pre-migration
	// backup, aborting on failure) before serving (technical/08 §1–§2).
	db, err := repo.Open(filepath.Join(cfg.DataDir, "econome.db"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(cfg.DataDir, "backups")); err != nil {
		return fmt.Errorf("econome: migrations: %w", err)
	}
	slog.Info("database ready")

	// Wire the layers: repo -> service; catalog -> renderer; then the server.
	store := repo.New(db)
	catalog, err := i18n.Load()
	if err != nil {
		return err
	}
	rdr, err := view.New(catalog)
	if err != nil {
		return err
	}
	svc := services.New(services.Deps{
		Users:        store.Users,
		Sessions:     store.Sessions,
		Settings:     store.Settings,
		Accounts:     store.Accounts,
		Categories:   store.Categories,
		Envelopes:    store.Envelopes,
		Allocations:  store.Allocations,
		Transactions: store.Transactions,
		Snapshots:    store.Snapshots,
		Periods:      store.Periods,
		Tx:           store,
		Secret:       secret,
	})

	srv := server.New(cfg, svc, rdr)

	// Run the server until a termination signal, then shut down gracefully.
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("econome shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// initLogging configures structured JSON logging to stdout at the configured
// level (technical/07 §8).
func initLogging(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}
