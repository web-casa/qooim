package console

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CSRF strategy: double-submit cookie + Origin/Host check.
//
//   - On every GET we ensure a `qooim_console_csrf` cookie exists
//     (mints a fresh 32-byte hex token if absent). The cookie is NOT
//     HttpOnly so the layout's Alpine snippet can read it and set the
//     HX-Headers global on the <body>, which makes every htmx request
//     send `X-CSRF-Token: <cookie value>`.
//
//   - On mutating verbs (POST/PUT/PATCH/DELETE) requireCSRF compares
//     the incoming header (or `csrf` form field) against the cookie
//     using constant-time compare. A separate Origin/Host check
//     hardens against the rare CSRF case where an attacker can plant
//     both cookie and header (e.g. via subdomain misconfig).
//
// The login flow uses the same cookie: GET /login mints, POST /login
// verifies + rotates so the post-login session can't replay the pre-
// login token.
const (
	csrfCookie = "qooim_console_csrf"
	csrfHeader = "X-CSRF-Token"
	csrfForm   = "csrf"
	// 12h matches the JWT cookie TTL we set on login.
	csrfDefaultTTL = 12 * 60 * 60
)

// ensureCSRFCookie returns the existing token or mints + sets a new
// one. Callers that build a View should stash the return value into
// View.CSRFToken so the layout can render it.
func (s *Server) ensureCSRFCookie(c *gin.Context) string {
	if v, err := c.Cookie(csrfCookie); err == nil && len(v) >= 32 {
		return v
	}
	tok := newCSRFToken()
	c.SetSameSite(http.SameSiteLaxMode)
	// secure=secureCookies, httpOnly=false (Alpine snippet must read it).
	c.SetCookie(csrfCookie, tok, csrfDefaultTTL, "/console", "", s.secureCookies, false)
	return tok
}

func (s *Server) rotateCSRFCookie(c *gin.Context) string {
	tok := newCSRFToken()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(csrfCookie, tok, csrfDefaultTTL, "/console", "", s.secureCookies, false)
	return tok
}

func (s *Server) clearCSRFCookie(c *gin.Context) {
	c.SetCookie(csrfCookie, "", -1, "/console", "", s.secureCookies, false)
}

func newCSRFToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// requireCSRF blocks any mutating request whose CSRF token doesn't
// match the cookie OR whose Origin (when present) doesn't match Host.
// GET/HEAD pass straight through.
func (s *Server) requireCSRF(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		c.Next()
		return
	}
	cookie, err := c.Cookie(csrfCookie)
	if err != nil || cookie == "" {
		csrfDeny(c, "CSRF cookie missing")
		return
	}
	got := c.GetHeader(csrfHeader)
	if got == "" {
		got = c.PostForm(csrfForm)
	}
	if got == "" {
		csrfDeny(c, "CSRF token missing")
		return
	}
	if subtle.ConstantTimeCompare([]byte(cookie), []byte(got)) != 1 {
		csrfDeny(c, "CSRF token mismatch")
		return
	}
	// Origin check (defense in depth). Browsers always send Origin on
	// CORS-relevant verbs; if it's there and doesn't match the Host we
	// reject regardless of the token. Same-origin form posts may omit
	// Origin in some older browsers, so absence isn't fatal.
	if origin := c.GetHeader("Origin"); origin != "" {
		if origin != "http://"+c.Request.Host && origin != "https://"+c.Request.Host {
			csrfDeny(c, "CSRF origin mismatch")
			return
		}
	}
	c.Next()
}

func csrfDeny(c *gin.Context, msg string) {
	c.String(http.StatusForbidden, msg)
	c.Abort()
}
