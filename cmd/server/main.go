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
