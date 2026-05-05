package api

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/config"
	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
	"github.com/web-casa/qooim/internal/storage"
)

type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *sql.DB
	jwt    *auth.Issuer
	store  storage.Storage

	q         db.Querier
	auth      *service.AuthService
	listing   *service.ListingService
	projects  *service.ProjectService
	repos     *service.RepoService
	templates *service.TemplateService
	files     *service.FileService
	surveys   *service.SurveyService
	answers   *service.AnswerService
	reports   *service.ReportService

	engine *gin.Engine
}

// NewServer wires routes, middleware, and services. db may be nil (skeleton
// mode); routes that require it will short-circuit when called. The
// storage backend is built from cfg; failures bubble out as a hard error
// since uploads are P2 functionality.
func NewServer(cfg *config.Config, logger *slog.Logger, sqlDB *sql.DB, jwt *auth.Issuer) (*Server, error) {
	if cfg.App.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	store, err := buildStorage(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("storage init: %w", err)
	}
	s := &Server{
		cfg:    cfg,
		logger: logger,
		db:     sqlDB,
		jwt:    jwt,
		store:  store,
		engine: gin.New(),
	}
	if sqlDB != nil {
		s.q = db.New(sqlDB)
		s.auth = service.NewAuthService(s.q, jwt)
		s.listing = service.NewListingService(s.q)
		s.projects = service.NewProjectService(s.q)
		s.repos = service.NewRepoService(s.q)
		s.templates = service.NewTemplateService(s.q)
		s.files = service.NewFileService(s.q, store)
		s.surveys = service.NewSurveyService(s.q)
		s.answers = service.NewAnswerService(s.q, s.surveys)
		s.reports = service.NewReportService(s.q)
	}
	s.engine.Use(gin.Recovery(), requestLogger(logger))
	s.routes()
	return s, nil
}

func buildStorage(cfg config.Storage) (storage.Storage, error) {
	switch cfg.Backend {
	case "", "local":
		return storage.NewLocal(cfg.LocalRoot)
	default:
		return nil, fmt.Errorf("unsupported storage backend %q", cfg.Backend)
	}
}

func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) routes() {
	s.engine.GET("/healthz", s.handleHealthz)
	s.engine.GET("/readyz", s.handleReadyz)

	api := s.engine.Group(s.cfg.HTTP.APIPrefix)
	{
		api.GET("/version", s.handleVersion)
		api.POST("/auth/login", s.requireDB, s.handleLogin)

		// Public survey rendering + answer submission. The "?t=<uid>"
		// query param is an opt-in partner token.
		api.GET("/survey/:projectId", s.requireDB, s.handleGetPublicSurvey)
		api.POST("/survey/:projectId/answer", s.requireDB, s.handleSubmitAnswer)
	}

	authed := api.Group("", s.requireDB, s.jwt.Middleware())
	{
		authed.GET("/me", s.handleMe)

		// Listings (P1).
		authed.GET("/projects", s.handleListProjects)
		authed.GET("/repos", s.handleListRepos)
		authed.GET("/templates", s.handleListTemplates)
		authed.GET("/dashboards", s.handleListDashboards)

		// Projects (P2 CRUD).
		authed.POST("/projects", s.handleCreateProject)
		authed.GET("/projects/:id", s.handleGetProject)
		authed.PUT("/projects/:id", s.handleUpdateProject)
		authed.DELETE("/projects/:id", s.handleDeleteProject)

		// Repos (P2 CRUD).
		authed.POST("/repos", s.handleCreateRepo)
		authed.GET("/repos/:id", s.handleGetRepo)
		authed.PUT("/repos/:id", s.handleUpdateRepo)
		authed.DELETE("/repos/:id", s.handleDeleteRepo)

		// Templates (P2 CRUD).
		authed.POST("/templates", s.handleCreateTemplate)
		authed.GET("/templates/:id", s.handleGetTemplate)
		authed.PUT("/templates/:id", s.handleUpdateTemplate)
		authed.DELETE("/templates/:id", s.handleDeleteTemplate)

		// Files (P2: local-disk upload + signed-less download).
		authed.POST("/files", s.handleUploadFile)
		authed.GET("/files/:id", s.handleDownloadFile)
		authed.DELETE("/files/:id", s.handleDeleteFile)

		// Answers (P3, admin-side).
		authed.GET("/projects/:id/answers", s.handleListAnswersByProject)
		authed.GET("/answers/:id", s.handleGetAnswer)
		authed.DELETE("/answers/:id", s.handleDeleteAnswer)

		// Reports / exports / exercise overview (P4).
		authed.GET("/projects/:id/report", s.handleProjectReport)
		authed.GET("/projects/:id/answers.xlsx", s.handleExportProjectAnswers)
		authed.POST("/repos/:id/templates/import", s.handleImportTemplates)
		authed.GET("/exercises", s.handleListExercises)
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
