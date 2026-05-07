// Package answerui implements the public-facing answer page.
package answerui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/service"
)

// Server bundles the answer renderer's deps. Mounted from the parent
// api.Server alongside /api/public/saveAnswer (which we still POST to
// for the actual write — keeps the storage layer in one place).
type Server struct {
	surveys  *service.SurveyService
	log      *slog.Logger
	demoMode bool
	tpl      *template.Template
}

// Deps is the slice of the parent server we depend on.
type Deps struct {
	Surveys *service.SurveyService
	Logger  *slog.Logger
	// DemoMode: when the requested project has no published survey,
	// fall back to a hard-coded two-question fixture so the renderer
	// can be smoke-tested without a real survey author. Off in prod.
	DemoMode bool
}

// Mount registers the public answer routes on the given engine.
func Mount(r gin.IRouter, deps Deps) {
	s := &Server{
		surveys:  deps.Surveys,
		log:      deps.Logger,
		demoMode: deps.DemoMode,
		tpl:      mustParseTemplates(),
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	r.GET("/answerui/static/*path", s.serveStatic)
	// SK already owns /answer in some bundles, so we use /answerui as
	// the renderer's namespace. Production routing can later 302
	// /answer/:projectId → /answerui/:projectId once the SK answer
	// page is retired.
	r.GET("/answerui/:projectId", s.getAnswer)
}

// ---- templates -------------------------------------------------------------

func mustParseTemplates() *template.Template {
	tplFS, err := fs.Sub(FS, "templates")
	if err != nil {
		panic(fmt.Errorf("answerui: sub templates: %w", err))
	}
	t := template.New("answer").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	})
	walkErr := fs.WalkDir(tplFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		raw, err := fs.ReadFile(tplFS, p)
		if err != nil {
			return err
		}
		_, err = t.New(p).Parse(string(raw))
		return err
	})
	if walkErr != nil {
		panic(fmt.Errorf("answerui: parse templates: %w", walkErr))
	}
	return t
}

// ---- model -----------------------------------------------------------------

// answerSurvey is the simplified schema this spike understands. The
// SK survey JSON is far richer (branches, scoring rules, conditional
// triggers); for Gate 4 we only need radio + upload to validate the
// contract — full SK compatibility is the Phase-3a goal.
type answerSurvey struct {
	Title     string              `json:"title"`
	Questions []answerQuestion    `json:"questions"`
	Setting   answerSurveySetting `json:"setting,omitempty"`
}

type answerSurveySetting struct {
	AllowSave bool `json:"allowSave,omitempty"`
}

type answerQuestion struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // "radio" | "upload"
	Title    string                 `json:"title"`
	Required bool                   `json:"required"`
	Options  []answerQuestionOption `json:"options,omitempty"`
	Accept   string                 `json:"accept,omitempty"`
}

type answerQuestionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// view is the template payload.
type view struct {
	Title     string
	ProjectID string
	Survey    *answerSurvey
	NotFound  bool
	ErrorMsg  string
	IsMobile  bool
}

// ---- handlers --------------------------------------------------------------

func (s *Server) getAnswer(c *gin.Context) {
	pid := c.Param("projectId")
	v := view{
		ProjectID: pid,
		Title:     "答题",
		IsMobile:  isMobile(c.Request.UserAgent()),
	}
	// Demo mode is gated on an explicit `?demo=1` query param so that
	// hitting an arbitrary unknown id no longer silently shows the
	// fixture under that id's URL. The DemoMode build flag still
	// has to be on (dev only); prod should never serve demo content.
	wantDemo := s.demoMode && c.Query("demo") == "1"
	if wantDemo && pid == "demo" {
		v.Survey = demoSurvey()
		v.Title = v.Survey.Title
		s.render(c, http.StatusOK, v)
		return
	}
	survey, err := s.loadSurvey(c.Request.Context(), pid)
	if errors.Is(err, service.ErrNotFound) {
		v.NotFound = true
		s.render(c, http.StatusNotFound, v)
		return
	} else if err != nil {
		s.log.Error("answerui.load", "err", err, "pid", pid)
		v.ErrorMsg = "加载问卷失败"
		s.render(c, http.StatusInternalServerError, v)
		return
	}
	v.Survey = survey
	v.Title = survey.Title
	s.render(c, http.StatusOK, v)
}

func (s *Server) loadSurvey(ctx context.Context, projectID string) (*answerSurvey, error) {
	pub, err := s.surveys.GetPublic(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if pub.Survey == "" {
		return nil, service.ErrNotFound
	}
	var sk answerSurvey
	if err := json.Unmarshal([]byte(pub.Survey), &sk); err != nil {
		// Couldn't parse with our simplified schema. In prod we'd fall
		// back to SK's full schema; for the spike we surface as not-
		// found so the demo mode (or a 404) takes over.
		return nil, fmt.Errorf("survey schema mismatch: %w", err)
	}
	if sk.Title == "" {
		sk.Title = pub.Name
	}
	return &sk, nil
}

// demoSurvey is the fallback fixture used when DemoMode=true and the
// real survey is missing. Radio + Upload exercise the two flows the
// spike claims to demonstrate.
func demoSurvey() *answerSurvey {
	return &answerSurvey{
		Title: "Qoo.IM Console 答题器 spike",
		Questions: []answerQuestion{
			{
				ID:       "q1",
				Type:     "radio",
				Title:    "您对这次答题体验的整体评价是？",
				Required: true,
				Options: []answerQuestionOption{
					{Value: "5", Label: "非常好"},
					{Value: "4", Label: "好"},
					{Value: "3", Label: "一般"},
					{Value: "2", Label: "差"},
					{Value: "1", Label: "非常差"},
				},
			},
			{
				ID:       "q2",
				Type:     "upload",
				Title:    "请上传一份反馈（图片/PDF）",
				Required: false,
				Accept:   ".pdf,.png,.jpg,.jpeg,.docx",
			},
		},
		Setting: answerSurveySetting{AllowSave: true},
	}
}

// answerUICSP is the baseline Content-Security-Policy for the public
// answer page. Same posture as the console (Alpine + HTMX + inline
// scripts), but the answer page is reachable WITHOUT auth so a strict
// CSP matters more — any XSS would land on a participant's anonymous
// session. 'unsafe-inline' + 'unsafe-eval' remain the Alpine
// concession for the spike; switch to nonces when the question-type
// renderer matures.
const answerUICSP = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// render shells out to the single page template — the spike has only
// one HTML page, so there's no name ambiguity. Future flow pages
// (e.g. /answerui/:projectId/done) can re-introduce a name parameter.
func (s *Server) render(c *gin.Context, status int, v view) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Security-Policy", answerUICSP)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "same-origin")
	if err := s.tpl.ExecuteTemplate(c.Writer, "answer.html", v); err != nil {
		s.log.Error("answerui.render", "err", err)
	}
}

// ---- static ---------------------------------------------------------------

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

// isMobile is a cheap UA sniff. We use it for layout density
// adjustments only — never for security or feature gating, since UA
// is trivially spoofed.
func isMobile(ua string) bool {
	ua = strings.ToLower(ua)
	for _, hint := range []string{"android", "iphone", "ipad", "ipod", "mobile"} {
		if strings.Contains(ua, hint) {
			return true
		}
	}
	return false
}
