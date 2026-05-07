package console

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

const (
	sessionCookie   = "qooim_console_session"
	flashKindError  = "error"
	flashKindOK     = ""
	defaultPageSize = 20
)

// Server bundles every dep the console handlers need. It is wired in
// Server.routes() (the main API server) via Mount.
type Server struct {
	authSvc *service.AuthService
	sysSvc  *service.SystemService
	jwt     *auth.Issuer
	q       db.Querier
	tpl     *templateBundle
	rawDB   *sql.DB
	// secureCookies enables `Secure` on the session + CSRF cookies. Off
	// in dev (HTTP loopback), on in prod (terminating TLS upstream or
	// otherwise). Driven by cfg.App.Env at Mount time.
	secureCookies bool
}

// Mount registers /console/* routes on the given engine. Pass the
// shared services from the parent Server so we don't duplicate
// dependency wiring.
func Mount(r gin.IRouter, deps Deps) {
	s := &Server{
		authSvc:       deps.Auth,
		sysSvc:        deps.System,
		jwt:           deps.JWT,
		q:             deps.Q,
		rawDB:         deps.RawDB,
		tpl:           mustParseTemplates(),
		secureCookies: deps.Env == "prod" || deps.Env == "production",
	}
	r.GET("/console/static/*path", s.serveStatic)

	g := r.Group("/console")
	{
		// Public. Login itself uses a "first-touch" CSRF cookie that
		// the GET handler primes; the POST handler verifies + rotates.
		g.GET("/login", s.getLogin)
		g.POST("/login", s.requireCSRF, s.postLogin)
		// Logout MUST be POST: a GET endpoint that clears state can
		// be triggered by an <img src> on a malicious page.
		g.POST("/logout", s.requireCSRF, s.logout)

		// Authed.
		authed := g.Group("", s.requireAuth)
		{
			authed.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/console/dashboard") })
			authed.GET("/dashboard", s.getDashboard)

			// Reads.
			authed.GET("/system/users", s.getUsers)
			authed.GET("/system/users/table", s.getUsersTable)
			authed.GET("/system/users/new", s.getUserForm)
			authed.GET("/system/users/:id/edit", s.getUserForm)

			// Writes — every mutation must clear CSRF. The middleware
			// also rejects any POST/PUT/DELETE whose Origin doesn't
			// match Host.
			mut := authed.Group("", s.requireCSRF)
			{
				mut.POST("/system/users", s.postUser)
				mut.PUT("/system/users/:id", s.putUser)
				mut.DELETE("/system/users/:id", s.deleteUser)
			}
		}
	}
}

// Deps is the slice of the parent Server we need.
type Deps struct {
	Auth   *service.AuthService
	System *service.SystemService
	JWT    *auth.Issuer
	Q      db.Querier
	RawDB  *sql.DB
	// Env mirrors cfg.App.Env so the console can flip Secure cookies
	// in prod without taking a dependency on the whole config struct.
	Env string
}

// ---- templates -------------------------------------------------------------

// templateBundle holds one fully-parsed template tree per page. Go's
// html/template puts every {{define "name"}} into a single global
// namespace — if every page file defines `content`, the last one wins
// and you ship the wrong UI. So we build a fresh tree per page that
// includes the shared layout + partials + that page's content.
type templateBundle struct {
	pages    map[string]*template.Template // by route key, e.g. "login.html"
	partials *template.Template            // standalone for HTMX fragment renders
}

// pageDefs lists every full-page template and the partials it depends
// on. Adding a page = one entry here.
var pageDefs = []struct {
	key   string   // route key passed to render()
	files []string // template files to parse together (order matters: layout first)
}{
	{"login.html", []string{"_layout.html", "login.html"}},
	{"dashboard.html", []string{"_layout.html", "dashboard.html"}},
	{"system/users/list.html", []string{
		"_layout.html",
		"system/users/list.html",
		"system/users/_table.html",
	}},
}

// partialFiles is the list of templates we parse into a separate tree
// for HTMX fragment swaps. Each file contributes its `{{define "name"}}`
// blocks (users-table, user-form, …).
var partialFiles = []string{
	"system/users/_table.html",
	"system/users/_form.html",
	"system/users/_refresh.html",
}

func mustParseTemplates() *templateBundle {
	funcs := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}
	tplFS, err := fs.Sub(FS, "templates")
	if err != nil {
		panic(fmt.Errorf("console: sub templates: %w", err))
	}
	pages := make(map[string]*template.Template, len(pageDefs))
	for _, def := range pageDefs {
		t := template.New(def.key).Funcs(funcs)
		for _, fn := range def.files {
			raw, err := fs.ReadFile(tplFS, fn)
			if err != nil {
				panic(fmt.Errorf("console: read %s: %w", fn, err))
			}
			if _, err := t.Parse(string(raw)); err != nil {
				panic(fmt.Errorf("console: parse %s: %w", fn, err))
			}
		}
		pages[def.key] = t
	}
	partials := template.New("partials").Funcs(funcs)
	for _, fn := range partialFiles {
		raw, err := fs.ReadFile(tplFS, fn)
		if err != nil {
			panic(fmt.Errorf("console: read partial %s: %w", fn, err))
		}
		if _, err := partials.Parse(string(raw)); err != nil {
			panic(fmt.Errorf("console: parse partial %s: %w", fn, err))
		}
	}
	return &templateBundle{pages: pages, partials: partials}
}

// View is the shared template payload. Every handler builds a View
// (sidebar/topbar context) and stuffs page-specific data into Data.
type View struct {
	Title     string
	Active    string
	Crumb     string
	Principal *auth.Principal
	Flash     *Flash
	Error     string
	// CSRFToken is rendered into every <form> as a hidden input AND
	// pinned on <body hx-headers='{"X-CSRF-Token":"…"}'> so HTMX-
	// driven requests carry it without the form.
	CSRFToken string

	// page-specific (template-defined fields read these directly)
	Username    string
	Stats       Stats
	Q           string
	Page        int
	TotalPages  int
	Total       int
	Rows        []userRow
	User        userForm
	Depts       []deptOption
	Roles       []roleOption
	UserRoleSet map[string]bool
}

type Flash struct {
	Kind    string
	Message string
}

type Stats struct{ Users, Projects, Repos, Answers int64 }

func (s *Server) render(c *gin.Context, name string, v View) {
	if v.Principal == nil {
		v.Principal = principalOf(c)
	}
	if v.CSRFToken == "" {
		v.CSRFToken = s.ensureCSRFCookie(c)
	}
	t, ok := s.tpl.pages[name]
	if !ok {
		c.String(http.StatusInternalServerError, "console: unknown page template %q", name)
		return
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	// Each page tree's root template is "layout" (defined in _layout.html);
	// page files inject `content` into that.
	if err := t.ExecuteTemplate(c.Writer, "layout", v); err != nil {
		_, _ = c.Writer.WriteString("<pre>render error: " + template.HTMLEscapeString(err.Error()) + "</pre>")
	}
}

// renderPartial executes a `{{define "name"}}` block directly — used
// for HTMX fragment swaps that must NOT include the layout chrome.
func (s *Server) renderPartial(c *gin.Context, name string, v View) {
	if v.CSRFToken == "" {
		v.CSRFToken = s.ensureCSRFCookie(c)
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.partials.ExecuteTemplate(c.Writer, name, v); err != nil {
		_, _ = c.Writer.WriteString("<pre>render error: " + template.HTMLEscapeString(err.Error()) + "</pre>")
	}
}

// ---- static ----------------------------------------------------------------

func (s *Server) serveStatic(c *gin.Context) {
	p := strings.TrimPrefix(c.Param("path"), "/")
	if p == "" || strings.Contains(p, "..") {
		c.Status(http.StatusNotFound)
		return
	}
	data, err := FS.ReadFile("static/" + p)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	switch {
	case strings.HasSuffix(p, ".css"):
		c.Header("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(p, ".js"):
		c.Header("Content-Type", "application/javascript; charset=utf-8")
	}
	c.Header("Cache-Control", "public, max-age=3600")
	_, _ = c.Writer.Write(data)
}

// ---- auth middleware -------------------------------------------------------

func (s *Server) requireAuth(c *gin.Context) {
	cookie, err := c.Cookie(sessionCookie)
	if err != nil || cookie == "" {
		c.Redirect(http.StatusFound, "/console/login")
		c.Abort()
		return
	}
	p, err := s.jwt.Parse(cookie)
	if err != nil {
		// expired/tampered — clear cookie and bounce.
		s.clearSession(c)
		c.Redirect(http.StatusFound, "/console/login")
		c.Abort()
		return
	}
	// Also gate on admin role; this matches what skAdmin enforces on
	// API routes. Non-admin can't reach the console at all.
	if !hasRole(p, "admin") {
		s.clearSession(c)
		c.String(http.StatusForbidden, "console requires admin role")
		c.Abort()
		return
	}
	c.Set(auth.ContextKey, p)
	c.Next()
}

func principalOf(c *gin.Context) *auth.Principal {
	if v, ok := c.Get(auth.ContextKey); ok {
		if p, ok := v.(*auth.Principal); ok {
			return p
		}
	}
	return nil
}

func hasRole(p *auth.Principal, role string) bool {
	if p == nil {
		return false
	}
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// ---- session helpers -------------------------------------------------------

func (s *Server) setSession(c *gin.Context, token string, ttlSeconds int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, ttlSeconds, "/console", "", s.secureCookies, true)
}

func (s *Server) clearSession(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/console", "", s.secureCookies, true)
}

// asError is a typed wrapper so handlers can check for service-layer
// not-found vs a 500 without spamming errors.Is everywhere.
func asError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "记录不存在"
	}
	return err.Error()
}
