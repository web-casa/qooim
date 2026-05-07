package console

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// sk_bridge.go is Gate 5 of the rewrite plan. Codex's review made the
// case for keeping the SurveyKing survey designer as a black-box rather
// than rewriting it ourselves — it's the most code-heavy, most tested
// part of SK and full parity is a 12-30 week effort. We keep it whole
// and only rewrite the chrome around it.
//
// The bridge solves the one stitching problem between our HTML-first
// console and SK's SPA: auth. Console stores its JWT in an HttpOnly
// cookie scoped to /console; SK's bundle reads it from localStorage
// under the key "Authorization" with a "Bearer <jwt>" value. So we
// surface a tiny "go to designer" route that:
//
//   1. Reads the console session JWT (server-side, since cookie is
//      HttpOnly).
//   2. Renders an inline-script page that writes localStorage and then
//      window.location's into the SK bundle.
//
// We keep the SK token write strictly server-driven: the JWT never
// crosses any URL or query string, only the inline-rendered page body.
// XSS would already get the cookie via parent-origin theft anyway, so
// localStorage isn't a meaningful additional surface.

func (s *Server) skBridge(c *gin.Context) {
	// requireAuth ran upstream so the cookie is valid; pull the
	// raw cookie value (the JWT itself) and inject it.
	cookie, err := c.Cookie(sessionCookie)
	if err != nil || cookie == "" {
		c.Redirect(http.StatusFound, "/console/login")
		return
	}
	// Parse it to get the username for the rendered greeting (also
	// validates the JWT didn't expire between requireAuth and now,
	// even though the gap is microseconds).
	p, err := s.jwt.Parse(cookie)
	if err != nil {
		c.Redirect(http.StatusFound, "/console/login")
		return
	}
	// `next` lets the caller specify where on SK to land — defaults
	// to the SPA root so SK shows its dashboard.
	next := c.Query("next")
	if next == "" || !isSafeNext(next) {
		next = "/"
	}
	v := bridgeView{
		Username:  p.Username,
		Token:     "Bearer " + cookie,
		Next:      next,
		ProjectID: c.Query("projectId"),
	}
	s.render(c, "sk-bridge.html", View{
		Title:     "前往 SK",
		Active:    "",
		Crumb:     "SK Bridge",
		CSRFToken: s.ensureCSRFCookie(c),
		Bridge:    &v,
	})
}

// isSafeNext keeps the bridge redirect inside our origin. We only
// accept paths starting with "/" and not "//" (which is a protocol-
// relative URL). This blocks an open-redirect via a manipulated
// `?next=https://evil.example/` query string.
func isSafeNext(p string) bool {
	if len(p) == 0 || p[0] != '/' {
		return false
	}
	if len(p) >= 2 && p[1] == '/' {
		return false
	}
	return true
}

type bridgeView struct {
	Username  string
	Token     string
	Next      string
	ProjectID string
}
