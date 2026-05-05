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

	"github.com/web-casa/qooim/internal/ai"
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

	api *api.Server
}

// SetAIProvider injects a Provider into the underlying api.Server.
// Pass nil to clear it (returns the route to the disabled-404 path).
func (s *Server) SetAIProvider(p ai.Provider) { s.api.SetAIProvider(p) }

// NewServer starts an in-process server with sensible test defaults.
// The caller owns Cleanup; t.Cleanup is registered automatically.
// Storage is rooted at a per-test temp dir so file-upload tests don't
// pollute each other.
func NewServer(t *testing.T, db *sql.DB) *Server {
	t.Helper()
	cfg := defaultTestConfig(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	issuer := auth.NewIssuer(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.ExpiresIn)
	srv, err := api.NewServer(cfg, logger, db, issuer)
	if err != nil {
		t.Fatalf("api.NewServer: %v", err)
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	return &Server{HTTP: hs, Cfg: cfg, DB: db, JWT: issuer, api: srv}
}

func (s *Server) URL(path string) string { return s.HTTP.URL + path }

func defaultTestConfig(t *testing.T) *config.Config {
	t.Helper()
	c, _ := config.Load("")
	c.App.Env = "test"
	c.JWT.Secret = "test-secret-do-not-use-in-prod"
	c.Storage.LocalRoot = t.TempDir()
	return c
}
