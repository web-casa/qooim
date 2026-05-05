package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
)

func (s *Server) handleListAnswersByProject(c *gin.Context) {
	res, err := s.answers.ListByProject(c.Request.Context(), c.Param("id"), parsePage(c))
	if err != nil {
		s.logger.Error("answers.list", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, res)
}

func (s *Server) handleGetAnswer(c *gin.Context) {
	row, err := s.answers.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, service.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "answer not found")
		return
	}
	if err != nil {
		s.logger.Error("answer.get", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, row)
}

func (s *Server) handleDeleteAnswer(c *gin.Context) {
	if err := s.answers.SoftDelete(c.Request.Context(), c.Param("id"), principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "answer not found")
			return
		}
		s.logger.Error("answer.delete", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}
