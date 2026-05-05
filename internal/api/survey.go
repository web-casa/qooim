package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
)

func (s *Server) handleGetPublicSurvey(c *gin.Context) {
	survey, err := s.surveys.GetPublic(c.Request.Context(), c.Param("projectId"))
	if errors.Is(err, service.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "survey not found or not published")
		return
	}
	if err != nil {
		s.logger.Error("survey.get", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, survey)
}

func (s *Server) handleSubmitAnswer(c *gin.Context) {
	var in service.SubmitInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "answer is required")
		return
	}

	meta := service.SubmitMeta{
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	if t := c.Query("t"); t != "" {
		// Best-effort partner lookup. A bad/missing token is allowed —
		// the answer is recorded as anonymous (create_by="guest").
		if p, err := s.surveys.LookupPartner(c.Request.Context(), t); err == nil {
			meta.Partner = p
		} else if !errors.Is(err, service.ErrNotFound) {
			s.logger.Warn("partner.lookup", "err", err)
		}
	}

	id, err := s.answers.Submit(c.Request.Context(), c.Param("projectId"), in, meta)
	if errors.Is(err, service.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "survey not found or not published")
		return
	}
	if err != nil {
		s.logger.Error("answer.submit", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}
