package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/web-casa/qooim/internal/api"
	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/config"
	"github.com/web-casa/qooim/internal/logger"
	"github.com/web-casa/qooim/internal/repo"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", os.Getenv("QOOIM_CONFIG"), "path to config file (yaml). overrides via QOOIM_* env vars.")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}

	log := logger.New(logger.Options{Level: cfg.Logger.Level, Format: cfg.Logger.Format})
	log.Info("starting", "name", cfg.App.Name, "env", cfg.App.Env, "version", cfg.App.Version, "addr", cfg.HTTP.Addr)

	db, err := openDB(cfg, log)
	if err != nil {
		return err
	}
	if db != nil {
		defer func() { _ = db.Close() }()
	}

	issuer := auth.NewIssuer(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.ExpiresIn)
	srv, err := api.NewServer(cfg, log, db, issuer)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      srv.Handler(),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	idleClosed := make(chan struct{})
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		log.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Error("shutdown", "err", err)
		}
		close(idleClosed)
	}()

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}
	<-idleClosed
	log.Info("stopped")
	return nil
}

// validateConfig fails fast on the misconfigurations that look fine at
// boot but blow up at the first request. We deliberately error rather
// than silently apply a default — leaving JWT secret empty in prod is
// the kind of bug we want to catch before the deploy.
func validateConfig(cfg *config.Config) error {
	if cfg.App.Env == "prod" || cfg.App.Env == "production" {
		if cfg.JWT.Secret == "" {
			return fmt.Errorf("config: jwt.secret must be set in env=%q (use QOOIM_JWT_SECRET)", cfg.App.Env)
		}
		if cfg.JWT.Secret == "change-me-in-production" {
			return fmt.Errorf("config: jwt.secret is the example default; set a real value via QOOIM_JWT_SECRET")
		}
	}
	if cfg.Storage.Backend == "" || cfg.Storage.Backend == "local" {
		root := cfg.Storage.LocalRoot
		if root == "" {
			return fmt.Errorf("config: storage.local_root is empty")
		}
		if abs, err := filepath.Abs(root); err == nil && abs != root {
			// Non-fatal: relative path resolves against CWD which on
			// systemd is "/". We log the absolute path that will actually
			// be used so operators can spot a wrong directory.
			cfg.Storage.LocalRoot = abs
		}
	}
	// Same treatment for the SPA bundle: a relative `./web/dist` resolves
	// against CWD, which is "/" under systemd. Promote to absolute and
	// fail fast if the directory is misconfigured.
	if cfg.HTTP.WebRoot != "" {
		abs, err := filepath.Abs(cfg.HTTP.WebRoot)
		if err != nil {
			return fmt.Errorf("config: http.web_root: %w", err)
		}
		cfg.HTTP.WebRoot = abs
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			// Don't error out — the operator may genuinely want API-only
			// mode but mistyped the path; prefer skipping the SPA over
			// blocking the whole server.
		}
	}
	return nil
}

// openDB returns (nil, nil) when no DSN is configured (skeleton mode for
// dev/tests). When a DSN is set we require it to actually connect — silently
// degrading would let a broken-config server pass /readyz and ship a lie.
func openDB(cfg *config.Config, log *slog.Logger) (*sql.DB, error) {
	if cfg.DB.DSN == "" {
		log.Warn("db.dsn is empty; running without database (dev/skeleton mode)")
		return nil, nil
	}
	db, err := repo.Open(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return db, nil
}
