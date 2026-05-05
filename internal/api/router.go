package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/config"
	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *sql.DB
	jwt    *auth.Issuer

	q       db.Querier
	auth    *service.AuthService
	listing *service.ListingService

	engine *gin.Engine
}

// NewServer wires routes, middleware, and services. db may be nil (skeleton
// mode); routes that require it will short-circuit when called.
func NewServer(cfg *config.Config, logger *slog.Logger, sqlDB *sql.DB, jwt *auth.Issuer) *Server {
	if cfg.App.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	s := &Server{
		cfg:    cfg,
		logger: logger,
		db:     sqlDB,
		jwt:    jwt,
		engine: gin.New(),
	}
	if sqlDB != nil {
		s.q = db.New(sqlDB)
		s.auth = service.NewAuthService(s.q, jwt)
		s.listing = service.NewListingService(s.q)
	}
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
		api.GET("/version", s.handleVersion)
		api.POST("/auth/login", s.requireDB, s.handleLogin)
	}

	authed := api.Group("", s.requireDB, s.jwt.Middleware())
	{
		authed.GET("/me", s.handleMe)
		authed.GET("/projects", s.handleListProjects)
		authed.GET("/repos", s.handleListRepos)
		authed.GET("/templates", s.handleListTemplates)
		authed.GET("/dashboards", s.handleListDashboards)
	}
}

// requireDB short-circuits routes that need persistence when the server was
// started in skeleton mode (no DSN configured). It must run before any
// auth/business middleware.
func (s *Server) requireDB(c *gin.Context) {
	if s.db == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "db_unavailable", "server started without a database; configure QOOIM_DB_DSN")
		return
	}
	c.Next()
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
