package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
)

type loginRequest struct {
	Account  string `json:"account"  binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "account and password are required")
		return
	}
	res, err := s.auth.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		if service.IsBadCredentials(err) {
			s.logger.Info("login.failed", "account", req.Account)
			httpx.Unauthorized(c, "invalid credentials")
			return
		}
		s.logger.Error("login.error", "account", req.Account, "err", err)
		httpx.Internal(c, "")
		return
	}
	s.logger.Info("login.ok", "account", req.Account, "user_id", res.Principal.UserID)
	c.JSON(http.StatusOK, res)
}

func (s *Server) handleMe(c *gin.Context) {
	p, ok := auth.FromContext(c)
	if !ok {
		httpx.Unauthorized(c, "")
		return
	}
	res, err := s.auth.Me(c.Request.Context(), *p)
	if err != nil {
		s.logger.Error("me.error", "user_id", p.UserID, "err", err)
		httpx.Internal(c, "")
		return
	}
	c.JSON(http.StatusOK, res)
}
