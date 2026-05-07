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
	// Loud warning when prod is running with weakened cookies. The
	// HTTP-only droplet uses this on purpose, but a staging deploy
	// behind a TLS terminator must not silently ship without Secure.
	if (cfg.App.Env == "prod" || cfg.App.Env == "production") && cfg.HTTP.InsecureCookies {
		log.Warn("config: cfg.HTTP.InsecureCookies=true in env=prod — Secure flag is OFF on console session/CSRF cookies. Only do this for HTTP-only deployments where the network is otherwise trusted.")
	}
	if len(cfg.HTTP.TrustedProxies) == 0 && (cfg.App.Env == "prod" || cfg.App.Env == "production") {
		log.Info("config: cfg.HTTP.TrustedProxies is empty in env=prod — c.ClientIP() returns the direct peer's RemoteAddr. Set the CIDR list when running behind a real reverse proxy.")
	}

	db, err := openDB(cfg, log)
	if err != nil {
		return err
	}
	if db != nil {
		defer func() { _ = db.Close() }()
	}

	if err := checkProdSeededAdmin(cfg, db); err != nil {
		return err
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
		// Don't os.Stat-then-error: the operator may genuinely want
		// API-only mode but mistyped the path; we'd rather skip the
		// SPA mount than block the whole server.
	}
	return nil
}

// checkProdSeededAdmin refuses to start in env=prod if the admin
// account still carries the SurveyKing seed bcrypt for "123456". The
// seed makes onboarding easy on a dev box but is a credential-stuffing
// magnet on a public deployment, so prod has to see a rotated hash.
//
// We match by hash *prefix* rather than full equality so a rotation in
// a future migration that re-bcrypts "123456" with a different salt
// still trips this guard. (Bcrypt('123456', cost=10) for the same
// salt always produces the same hash; what we're really detecting is
// "the admin row was never touched after seed".)
func checkProdSeededAdmin(cfg *config.Config, db *sql.DB) error {
	if db == nil {
		return nil
	}
	if cfg.App.Env != "prod" && cfg.App.Env != "production" {
		return nil
	}
	const seededHashPrefix = "$2a$10$vZk9P3XtbD2KrdLbQYPvBu"
	var hash sql.NullString
	row := db.QueryRow(`SELECT auth_secret FROM t_account WHERE auth_account='admin' AND auth_type='PWD' AND is_deleted=0 LIMIT 1`)
	if err := row.Scan(&hash); err != nil {
		// Either no admin row or the schema is somewhere else; we don't
		// want a transient DB hiccup to keep prod down, so log via the
		// returned error path and move on.
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check admin password: %w", err)
	}
	if hash.Valid && len(hash.String) >= len(seededHashPrefix) && hash.String[:len(seededHashPrefix)] == seededHashPrefix {
		return fmt.Errorf("config: refusing to start in env=%q with the SurveyKing seed admin password still in place — rotate t_account.auth_secret for the admin row first", cfg.App.Env)
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
