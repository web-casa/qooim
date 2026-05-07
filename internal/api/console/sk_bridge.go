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
	// The bridge response carries a Bearer token in its body. Even
	// though the route is admin-only and behind cookie auth, a
	// well-meaning intermediary cache (CDN, corp proxy, browser bf-
	// cache) could store it and re-serve it to the next user on the
	// same shared connection. `no-store` keeps it out of every
	// cache; `no-cache` would still allow an Etag round-trip.
	c.Header("Cache-Control", "no-store, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	s.render(c, "sk-bridge.html", View{
		Title:     "前往 SK",
		Active:    "",
		Crumb:     "SK Bridge",
		CSRFToken: s.ensureCSRFCookie(c),
		Bridge:    &v,
	})
}

// isSafeNext keeps the bridge redirect inside our origin. The
// open-redirect family of bypasses all turn on "looks like a path
// but a browser parses it as a host". We block:
//
//	"//evil"            protocol-relative URL
//	"/\evil"            backslash that legacy IE coerces to "/"
//	"/<whitespace>//x"  some browsers strip leading whitespace before
//	                    re-parsing the URL, and this would resolve as
//	                    "//x"
//
// Anything starting with "/" followed by an unambiguous path char
// passes. We deliberately don't try to be clever about what's a
// "valid path" — the browser will resolve the URL relative to our
// origin and a hostile path that contains evil bytes will get URL-
// escaped, not honoured as a host.
func isSafeNext(p string) bool {
	if len(p) == 0 || p[0] != '/' {
		return false
	}
	if len(p) >= 2 {
		switch p[1] {
		case '/', '\\', '\t', '\n', '\r', ' ':
			return false
		}
	}
	return true
}

type bridgeView struct {
	Username  string
	Token     string
	Next      string
	ProjectID string
}
