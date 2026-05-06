package api

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/ai"
	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/config"
	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
	"github.com/web-casa/qooim/internal/storage"
)

type Server struct {
	cfg           *config.Config
	logger        *slog.Logger
	db            *sql.DB
	jwt           *auth.Issuer
	loginKP       *auth.LoginKeyPair
	store         storage.Storage
	publicLoginRL *rateLimiter
	publicWriteRL *rateLimiter

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
	aiSvc     *service.AIService
	system    *service.SystemService

	engine *gin.Engine
}

// SetAIProvider swaps the provider after NewServer has registered
// routes. This is safe: the /api/ai/chat handler resolves s.aiSvc on
// each request, so a mid-life replacement is observed by the next call
// without a server restart. Used by tests; production code wires the
// provider in NewServer based on cfg.AI.
func (s *Server) SetAIProvider(p ai.Provider) {
	if p == nil {
		s.aiSvc = nil
		return
	}
	s.aiSvc = service.NewAIService(p)
}

// NewServer wires routes, middleware, and services. db may be nil (skeleton
// mode); routes that require it will short-circuit when called. The
// storage backend is built from cfg; failures bubble out as a hard error
// since uploads are P2 functionality.
func NewServer(cfg *config.Config, logger *slog.Logger, sqlDB *sql.DB, jwt *auth.Issuer) (*Server, error) {
	if cfg.App.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	loginKP, err := auth.DefaultLoginKeyPair()
	if err != nil {
		return nil, fmt.Errorf("init login keypair: %w", err)
	}
	store, err := buildStorage(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("storage init: %w", err)
	}
	s := &Server{
		cfg:     cfg,
		logger:  logger,
		db:      sqlDB,
		jwt:     jwt,
		loginKP: loginKP,
		store:   store,
		engine:  gin.New(),
		// Login: ~5/sec drip with a 10-burst per IP — enough headroom
		// for a real human typo but not for a credential-stuffing run.
		// saveAnswer/upload share a more relaxed bucket since their
		// legitimate use is per-survey.
		publicLoginRL: newRateLimiter(5, 10, 5*time.Minute),
		publicWriteRL: newRateLimiter(20, 40, 5*time.Minute),
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
		s.system = service.NewSystemService(s.q, sqlDB)
	}
	if cfg.AI.Enabled && cfg.AI.Token != "" {
		provider := ai.NewOpenAICompatible(
			cfg.AI.Provider,
			cfg.AI.BaseURL,
			cfg.AI.Token,
			cfg.AI.Model,
			cfg.AI.HTTPTimeout,
		)
		s.aiSvc = service.NewAIService(provider)
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
		api.POST("/auth/login", s.requireDB, rateLimit(s.publicLoginRL), s.handleLogin)

		// Public survey rendering + answer submission. The "?t=<uid>"
		// query param is an opt-in partner token. Answer submit is
		// rate-limited so the open POST isn't a free DDoS.
		api.GET("/survey/:projectId", s.requireDB, s.handleGetPublicSurvey)
		api.POST("/survey/:projectId/answer", s.requireDB, rateLimit(s.publicWriteRL), s.handleSubmitAnswer)
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

		// AI chat (P5). The handler 404s when no provider is configured
		// so the existence of the feature isn't leaked.
		authed.POST("/ai/chat", s.handleAIChat)
	}

	// ----- SK-compat adapter (C1 + C2) -----
	// SK frontend speaks action-style routes with a {success,code:200,
	// data,total?} envelope. Public flow uses no auth; admin flow uses
	// a SK-shape JWT middleware so 401s trigger the bundle's auto-logout.
	api.POST("/public/login", s.requireDB, rateLimit(s.publicLoginRL), s.handleSKLogin)
	api.POST("/public/logout", s.handleSKLogout)
	// Public answer flow (C2). Participant identity rides on a partner
	// `?token=<uid>` query — best-effort, anonymous fallback.
	api.POST("/public/loadProject", s.requireDB, s.handleSKLoadProject)
	api.POST("/public/validateProject", s.requireDB, s.handleSKValidateProject)
	api.POST("/public/saveAnswer", s.requireDB, rateLimit(s.publicWriteRL), s.handleSKSaveAnswer)
	api.POST("/public/upload", s.requireDB, rateLimit(s.publicWriteRL), s.handleSKPublicUpload)

	skAuthed := api.Group("", s.requireDB, s.skJWTMiddleware())
	skAdmin := api.Group("", s.requireDB, s.skJWTMiddleware(), s.requireAdmin())
	_ = skAdmin // referenced inside the block below
	{
		skAuthed.GET("/currentUser", s.handleSKCurrentUser)

		// Project (C1). SK actually uses GET /api/project/list with query
		// string; we keep POST too because earlier C1 already shipped it
		// and some screens may not have been re-bundled.
		skAuthed.GET("/project/list", s.handleSKProjectList)
		skAuthed.POST("/project/list", s.handleSKProjectList)
		skAuthed.GET("/project", s.handleSKProjectGet)
		skAdmin.POST("/project/create", s.handleSKProjectCreate)
		skAdmin.POST("/project/update", s.handleSKProjectUpdate)
		skAdmin.POST("/project/delete", s.handleSKProjectDelete)

		// Template (C2).
		skAuthed.GET("/template/list", s.handleSKTemplateList)
		skAuthed.GET("/template/get", s.handleSKTemplateGet)
		skAuthed.POST("/template/get", s.handleSKTemplateGet)
		skAdmin.POST("/template/create", s.handleSKTemplateCreate)
		skAdmin.POST("/template/update", s.handleSKTemplateUpdate)
		skAdmin.POST("/template/delete", s.handleSKTemplateDelete)
		skAuthed.GET("/template/listCategory", s.handleSKTemplateListCategory)
		skAuthed.GET("/template/listTag", s.handleSKTemplateListTag)

		// Repo (C2).
		skAuthed.GET("/repo/list", s.handleSKRepoList)
		skAuthed.GET("/repo", s.handleSKRepoGet)
		skAdmin.POST("/repo/create", s.handleSKRepoCreate)
		skAdmin.POST("/repo/update", s.handleSKRepoUpdate)
		skAdmin.POST("/repo/delete", s.handleSKRepoDelete)

		// File (C2 admin paths). Upload + delete + list need a JWT.
		skAdmin.POST("/file/create", s.handleSKFileCreate)
		skAuthed.GET("/file/list", s.handleSKFileList)
		skAdmin.POST("/file/delete", s.handleSKFileDelete)

		// System admin (C3) — dept/role/user/position/dict + sysinfo.
		// /system is registered on the public api group below — needs
		// to be reachable BEFORE login since the SPA reads publicKey
		// from there to encrypt the login password.
		skAdmin.POST("/system/update", s.handleSKSystemUpdate)
		skAdmin.GET("/system/aiSetting", s.handleSKAiSetting)
		skAdmin.GET("/system/permission/list", s.handleSKPermissionList)
		skAdmin.GET("/system/checkUsernameExist", s.handleSKCheckUsername)

		skAdmin.GET("/system/dept/list", s.handleSKDeptList)
		skAdmin.POST("/system/dept/create", s.handleSKDeptCreate)
		skAdmin.POST("/system/dept/update", s.handleSKDeptUpdate)
		skAdmin.POST("/system/dept/delete", s.handleSKDeptDelete)

		skAdmin.GET("/system/role/list", s.handleSKRoleList)
		skAdmin.POST("/system/role/create", s.handleSKRoleCreate)
		skAdmin.POST("/system/role/update", s.handleSKRoleUpdate)
		skAdmin.POST("/system/role/delete", s.handleSKRoleDelete)

		skAdmin.GET("/system/user/list", s.handleSKUserList)
		skAdmin.POST("/system/user/create", s.handleSKUserCreate)
		skAdmin.POST("/system/user/update", s.handleSKUserUpdate)
		skAdmin.POST("/system/user/delete", s.handleSKUserDelete)

		skAdmin.GET("/system/position/list", s.handleSKPositionList)
		skAdmin.POST("/system/position/create", s.handleSKPositionCreate)
		skAdmin.POST("/system/position/update", s.handleSKPositionUpdate)
		skAdmin.POST("/system/position/delete", s.handleSKPositionDelete)

		skAdmin.GET("/system/dict/list", s.handleSKDictList)
		skAdmin.POST("/system/dict/create", s.handleSKDictCreate)
		skAdmin.POST("/system/dict/update", s.handleSKDictUpdate)
		skAdmin.POST("/system/dict/delete", s.handleSKDictDelete)

		skAdmin.GET("/system/dictItem/list", s.handleSKDictItemList)
		skAdmin.POST("/system/dictItem/create", s.handleSKDictItemCreate)
		skAdmin.POST("/system/dictItem/update", s.handleSKDictItemUpdate)
		skAdmin.POST("/system/dictItem/delete", s.handleSKDictItemDelete)

		// Dashboard / overview (C4).
		skAdmin.GET("/userOverview", s.handleSKUserOverview)
		skAuthed.GET("/exercise/list", s.handleSKExerciseList)

		// Answer admin (C4).
		skAuthed.GET("/answer/list", s.handleSKAnswerList)
		skAdmin.GET("/answer/trash", s.handleSKAnswerTrash)
		skAdmin.POST("/answer/create", s.handleSKAnswerCreate)
		skAdmin.POST("/answer/delete", s.handleSKAnswerDelete)
		skAdmin.POST("/answer/restore", s.handleSKAnswerRestore)
		skAdmin.POST("/answer/destroy", s.handleSKAnswerDestroy)
		skAdmin.POST("/answer/update", s.handleSKAnswerUpdate)
		skAdmin.GET("/answer/download", s.handleSKAnswerDownload)
		skAdmin.POST("/answer/upload", s.handleSKAnswerUpload)

		// Project report. SK calls /api/report/<projectId>?search=<term>
		// (admin-side report data) — separate from /api/projects/:id/report.
		skAdmin.GET("/report/:id", s.handleSKReport)

		// Project / repo partner (C4).
		skAdmin.GET("/project/partner/list", s.handleSKProjectPartnerList)
		skAdmin.POST("/project/partner/create", s.handleSKProjectPartnerCreate)
		skAdmin.POST("/project/partner/delete", s.handleSKProjectPartnerDelete)
		skAdmin.POST("/project/partner/import", s.handleSKProjectPartnerImport)
		skAdmin.GET("/project/partner/download", s.handleSKProjectPartnerDownload)
		skAdmin.GET("/repo/partner/list", s.handleSKRepoPartnerList)
		skAdmin.POST("/repo/partner/create", s.handleSKRepoPartnerCreate)
		skAdmin.POST("/repo/partner/delete", s.handleSKRepoPartnerDelete)

		// Repo book (per-user wrong-question / favourites bag, C4).
		skAuthed.GET("/repo/book/list", s.handleSKUserBookList)
		skAuthed.POST("/repo/book/create", s.handleSKUserBookCreate)
		skAuthed.POST("/repo/book/update", s.handleSKUserBookUpdate)
		skAuthed.POST("/repo/book/delete", s.handleSKUserBookDelete)

		// File template download (xlsx import templates).
		skAuthed.GET("/file/downloadTemplate", s.handleSKFileDownloadTemplate)

		// Workflow stubs — Flowable was dropped in P0 but the SK
		// frontend probes these on project edit; empty 200s keep the
		// UI from crashing.
		skAdmin.GET("/workflow/loadSchema", s.handleSKWorkflowEmptyObject)
		skAdmin.GET("/workflow/getFlow", s.handleSKWorkflowEmptyObject)
		skAdmin.POST("/workflow/saveFlow", s.handleSKWorkflowEmptyObject)
		skAdmin.GET("/workflow/getFlowTasks", s.handleSKWorkflowEmptyList)
		skAdmin.GET("/workflow/approvalTask", s.handleSKWorkflowEmptyObject)
		skAdmin.POST("/workflow/approvalTask", s.handleSKWorkflowEmptyObject)
		skAdmin.GET("/workflow/getAuditRecord", s.handleSKWorkflowEmptyList)
		skAdmin.GET("/workflow/getRevertNodes", s.handleSKWorkflowEmptyList)
		skAdmin.POST("/workflow/deploy", s.handleSKWorkflowEmptyObject)
		skAdmin.GET("/workflow/statics", s.handleSKWorkflowEmptyObject)
		skAdmin.GET("/listUserTask", s.handleSKWorkflowEmptyList)
		skAdmin.GET("/listHistoryTask", s.handleSKWorkflowEmptyList)

		// AI in SK shape (C4). 404 if no provider — same hidden-feature
		// contract as the REST /api/ai/chat.
		skAdmin.POST("/ai/chat/create-conversation", s.handleSKAIChatCreateConversation)
		skAdmin.POST("/ai/chat/close-conversation", s.handleSKAIChatCloseConversation)
		skAdmin.GET("/ai/chat/models", s.handleSKAIChatModels)
		skAdmin.POST("/ai/chat/stream", s.handleSKAIChatStream)
		skAdmin.POST("/ai/chat/answer-analysis/create-conversation", s.handleSKAIAnswerAnalysisCreate)
		skAdmin.POST("/ai/chat/answer-analysis/close-conversation", s.handleSKAIAnswerAnalysisClose)
		skAdmin.POST("/ai/chat/answer-analysis/stream", s.handleSKAIAnswerAnalysisStream)

		// C5: project trash bin.
		skAdmin.GET("/project/trash", s.handleSKProjectTrash)
		skAdmin.POST("/project/restore", s.handleSKProjectRestore)
		skAdmin.POST("/project/destroy", s.handleSKProjectDestroy)

		// C5: project edit screen pickers.
		// SK calls every selector via POST (umi `service.post(...)`),
		// but a GET form is harmless for ad-hoc curl/browser checks —
		// register both verbs so neither caller has to remember which.
		for _, verb := range []string{"GET", "POST"} {
			skAdmin.Handle(verb, "/project/selectDept", s.handleSKProjectSelectDept)
			skAdmin.Handle(verb, "/project/selectPosition", s.handleSKProjectSelectPosition)
			skAdmin.Handle(verb, "/project/selectRole", s.handleSKProjectSelectRole)
			skAdmin.Handle(verb, "/project/selectUser", s.handleSKProjectSelectUser)
			skAdmin.Handle(verb, "/project/selectRepo", s.handleSKProjectSelectRepo)
			skAdmin.Handle(verb, "/project/selectTemplate", s.handleSKProjectSelectTemplate)
			skAdmin.Handle(verb, "/project/selectTag", s.handleSKProjectSelectTag)
			skAdmin.Handle(verb, "/project/selectDict", s.handleSKProjectSelectDict)
		}

		// C5: dept drag-drop reorder + dictItem xlsx import.
		skAdmin.POST("/system/dept/sort", s.handleSKDeptSort)
		skAdmin.POST("/system/dictItem/import", s.handleSKDictItemImport)

		// C5: repo bulk operations.
		skAdmin.POST("/repo/batchCreate", s.handleSKRepoBatchCreate)
		skAdmin.GET("/repo/export", s.handleSKRepoExport)
		skAdmin.POST("/repo/import", s.handleSKRepoImport)
		skAdmin.POST("/repo/pick", s.handleSKRepoPick)
		skAdmin.POST("/repo/unbind", s.handleSKRepoUnbind)
	}

	// C5: self-registration. Public route — no JWT.
	api.POST("/public/register", s.requireDB, s.handleSKRegister)

	// /api/system needs to be reachable BEFORE login because the SK
	// frontend pulls system.publicKey from here to RSA-encrypt the
	// login form's password. The handler doesn't read the principal.
	api.GET("/system", s.requireDB, s.handleSKSystem)

	// /api/public/load* — these are unauthenticated helpers SK calls
	// from the answer / question-editor pages: dict lookup, saved
	// query loading, exam-result fetching, etc. We don't have a real
	// implementation for the historical SK semantics; empty 200s keep
	// the bundle from showing "网络连接失败" toasts.
	api.POST("/public/loadDict", s.requireDB, s.handleSKPublicLoadDict)
	api.POST("/public/loadQuery", s.handleSKPublicEmpty)
	api.POST("/public/getQueryResult", s.handleSKPublicEmpty)
	api.POST("/public/loadExamResult", s.handleSKPublicEmpty)
	api.POST("/public/loadLinkResult", s.handleSKPublicEmpty)
	api.GET("/public/listRegisterRole", s.handleSKPublicEmptyList)
	api.POST("/public/statistics", s.handleSKPublicEmpty)

	// Public file read — `<img src="/api/file?id=...">` cannot send the
	// Authorization header, so the read route must live outside JWT. The
	// `shared` column on t_file is the eventual gate; until that's wired
	// every uploaded file is publicly readable by id, matching SK.
	api.GET("/file", s.requireDB, s.handleSKFileGet)
	// SK avatars / image embeds use /api/public/preview/<id> (with an
	// optional "@thumbnail" suffix that we currently ignore — full
	// image is served instead). Maps onto the same backing service as
	// /api/file.
	api.GET("/public/preview/:id", s.requireDB, s.handleSKPublicPreview)

	// SPA + static files (must be last; uses NoRoute).
	s.installSPA(s.cfg.HTTP.WebRoot)
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
