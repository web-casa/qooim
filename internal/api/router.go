package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ivmm/exam-run/internal/auth"
	"github.com/ivmm/exam-run/internal/config"
	"github.com/ivmm/exam-run/internal/httpx"
)

type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *sql.DB
	jwt    *auth.Issuer
	engine *gin.Engine
}

func NewServer(cfg *config.Config, logger *slog.Logger, db *sql.DB, jwt *auth.Issuer) *Server {
	if cfg.App.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	s := &Server{cfg: cfg, logger: logger, db: db, jwt: jwt, engine: gin.New()}
	s.engine.Use(gin.Recovery(), requestLogger(logger))
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) routes() {
	s.engine.GET("/healthz", s.handleHealthz)
	s.engine.GET("/readyz", s.handleReadyz)

	api := s.engine.Group(s.cfg.HTTP.APIPrefix)
	{
		// Auth-free placeholders.
		api.GET("/version", s.handleVersion)
	}

	// Authenticated routes go here in P1+.
	// authed := api.Group("", s.jwt.Middleware())
	// _ = authed
}

func (s *Server) handleHealthz(c *gin.Context) {
	httpx.OK(c, gin.H{"status": "ok"})
}

func (s *Server) handleReadyz(c *gin.Context) {
	if s.db != nil {
		ctx, cancel := contextWithTimeout(c, 2*time.Second)
		defer cancel()
		if err := s.db.PingContext(ctx); err != nil {
			httpx.Error(c, http.StatusServiceUnavailable, "db_unavailable", err.Error())
			return
		}
	}
	httpx.OK(c, gin.H{"status": "ready"})
}

func (s *Server) handleVersion(c *gin.Context) {
	httpx.OK(c, gin.H{
		"name":    s.cfg.App.Name,
		"version": s.cfg.App.Version,
		"env":     s.cfg.App.Env,
	})
}
