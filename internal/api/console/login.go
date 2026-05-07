package console

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

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
	// 12h session, matches a typical admin shift. JWT itself carries
	// the real expiry; cookie max-age is just a UX hint to the browser.
	setSession(c, res.Token, 12*60*60)
	c.Redirect(http.StatusFound, "/console/dashboard")
}

func (s *Server) logout(c *gin.Context) {
	clearSession(c)
	c.Redirect(http.StatusFound, "/console/login")
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
