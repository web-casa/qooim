// Package testenv provides shared fixtures for end-to-end and integration
// tests: an in-process HTTP server, optional MySQL via dockertest, and a
// thin HTTP client.
package testenv

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/web-casa/qooim/internal/api"
	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/config"
)

// Server is an in-process Qoo.IM server backed by httptest.
// Pass DB=nil to test endpoints that don't require a database.
type Server struct {
	HTTP *httptest.Server
	Cfg  *config.Config
	DB   *sql.DB
	JWT  *auth.Issuer
}

// NewServer starts an in-process server with sensible test defaults.
// The caller owns Cleanup; t.Cleanup is registered automatically.
func NewServer(t *testing.T, db *sql.DB) *Server {
	t.Helper()
	cfg := defaultTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	issuer := auth.NewIssuer(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.ExpiresIn)
	srv := api.NewServer(cfg, logger, db, issuer)
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	return &Server{HTTP: hs, Cfg: cfg, DB: db, JWT: issuer}
}

func (s *Server) URL(path string) string { return s.HTTP.URL + path }

func defaultTestConfig() *config.Config {
	c, _ := config.Load("")
	c.App.Env = "test"
	c.JWT.Secret = "test-secret-do-not-use-in-prod"
	return c
}
