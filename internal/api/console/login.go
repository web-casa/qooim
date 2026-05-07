package console

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// consoleSessionTTL is the wall-clock lifetime of a console session.
// We mint the JWT with this TTL and set the cookie's MaxAge to match,
// so the bearer the SK bridge later writes into localStorage cannot
// outlive the cookie that authorised the bridge call. Codex Gate-5
// review #2 caught the previous mismatch (cookie 12h vs JWT 24h
// default).
const consoleSessionTTL = 12 * time.Hour

func (s *Server) getLogin(c *gin.Context) {
	// If already logged in, skip.
	if cookie, err := c.Cookie(sessionCookie); err == nil && cookie != "" {
		if _, err := s.jwt.Parse(cookie); err == nil {
			c.Redirect(http.StatusFound, "/console/dashboard")
			return
		}
	}
	s.render(c, "login.html", View{Title: "登录"})
}

func (s *Server) postLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	if username == "" || password == "" {
		s.render(c, "login.html", View{Title: "登录", Username: username, Error: "账号和密码必填"})
		return
	}
	res, err := s.authSvc.Login(c.Request.Context(), username, password)
	if err != nil {
		s.render(c, "login.html", View{Title: "登录", Username: username, Error: "账号或密码错误"})
		return
	}
	// Same admin-role gate as the middleware so a non-admin user can't
	// even open the login → 302 → 403 dance; show the error inline.
	if !hasRole(&res.Principal, "admin") {
		s.render(c, "login.html", View{Title: "登录", Username: username, Error: "Console 仅限管理员"})
		return
	}
	// Issue a console-scoped JWT with our own (shorter) TTL — the
	// authSvc default uses cfg.JWT.ExpiresIn which can be 24h+ and
	// would let a token that the SK bridge plants in localStorage
	// outlive the cookie that authorised it. Cookie + JWT now expire
	// at the same wall time.
	consoleToken, err := s.jwt.SignWithTTL(res.Principal, consoleSessionTTL)
	if err != nil {
		s.log.Error("console.login.sign", "err", err)
		s.render(c, "login.html", View{Title: "登录", Username: username, Error: "登录失败，请重试"})
		return
	}
	s.setSession(c, consoleToken, int(consoleSessionTTL.Seconds()))
	// Rotate the CSRF token so the post-login session can't replay the
	// pre-login token.
	s.rotateCSRFCookie(c)
	c.Redirect(http.StatusFound, "/console/dashboard")
}

func (s *Server) logout(c *gin.Context) {
	s.clearSession(c)
	s.clearCSRFCookie(c)
	// Render a page that runs `localStorage.removeItem` for the SK
	// bridge keys before redirecting. A bare 302 → /console/login
	// would leave the Bearer token sitting in localStorage where
	// any same-origin XSS could read it.
	c.Header("Cache-Control", "no-store")
	s.render(c, "logout.html", View{Title: "退出登录"})
}

func (s *Server) getDashboard(c *gin.Context) {
	stats := s.collectStats(c.Request.Context())
	s.render(c, "dashboard.html", View{
		Title:  "仪表盘",
		Active: "dashboard",
		Crumb:  "仪表盘",
		Stats:  stats,
	})
}

func (s *Server) collectStats(ctx context.Context) Stats {
	var st Stats
	// Best-effort: failures fall back to 0 — the dashboard is a glance,
	// not a source of truth, so a transient DB hiccup shouldn't 500.
	if s.rawDB == nil {
		return st
	}
	row := s.rawDB.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM t_user      WHERE is_deleted=0),
		(SELECT count(*) FROM t_project   WHERE is_deleted=0),
		(SELECT count(*) FROM t_repo      WHERE is_deleted=0),
		(SELECT count(*) FROM t_answer    WHERE is_deleted=0)`)
	_ = row.Scan(&st.Users, &st.Projects, &st.Repos, &st.Answers)
	return st
}
