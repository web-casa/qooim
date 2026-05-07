package console

import (
	"net/http"
	"net/url"
	"strings"

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
	// Defensive role check. requireAuth (one frame up) already
	// rejects non-admin sessions, but a future refactor that drops
	// the bridge into a less-restricted group would silently start
	// handing JWTs to viewers. Belt-and-braces: refuse explicitly.
	if !hasRole(p, "admin") {
		c.String(http.StatusForbidden, "console requires admin role")
		return
	}
	// `next` lets the caller specify where on SK to land — defaults
	// to the SPA root so SK shows its dashboard.
	next := c.Query("next")
	if next == "" || !isSafeNext(next) {
		next = "/"
	}
	v := bridgeView{
		Username: p.Username,
		// SK's request adapter reads localStorage.Authorization and
		// PREPENDS "Bearer " before sending — see umi.c1ebddb4.js:
		//   p(){var z=localStorage.getItem("Authorization");
		//      return z?"Bearer ".concat(z):""}
		// So we store the RAW JWT here. Putting "Bearer " in
		// localStorage causes SK to send "Bearer Bearer eyJ..." and
		// every authed call fails with "invalid or expired token".
		Token:     cookie,
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

// isSafeNext keeps the bridge redirect inside our origin. We allow a
// `next` value only when it parses as a relative URL (no scheme, no
// host) AND its path starts with "/" but not with a sequence the
// browser would reinterpret as a host:
//
//	"//evil"            protocol-relative URL
//	"/\evil"            backslash that legacy IE coerces to "/"
//	"/<whitespace>//x"  some browsers strip leading whitespace before
//	                    re-parsing the URL
//
// We also reject any `data:`, `javascript:`, etc. that slips through
// the leading-slash check (defence in depth — the URL parse already
// strips schemes, but we belt-and-brace).
func isSafeNext(p string) bool {
	if len(p) == 0 || p[0] != '/' {
		return false
	}
	// Reject the second-byte separators / ambiguous whitespace before
	// we even try to parse — strings like "/\\foo" parse fine but
	// browsers can rewrite them.
	if len(p) >= 2 {
		switch p[1] {
		case '/', '\\', '\t', '\n', '\r', ' ', '\v', '\f':
			return false
		}
	}
	u, err := url.Parse(p)
	if err != nil {
		return false
	}
	// Either of these means the parse picked up a non-path component;
	// browsers will follow that as a host change. The leading-byte
	// check above usually catches them but `url.Parse` is the
	// authoritative answer.
	if u.Scheme != "" || u.Host != "" || u.Opaque != "" {
		return false
	}
	// Path MUST start with "/" — relative paths like "foo/bar" would
	// resolve relative to the bridge URL and could traverse into a
	// surprising location.
	if !strings.HasPrefix(u.Path, "/") {
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
