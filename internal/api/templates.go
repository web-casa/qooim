package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/domain"
	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
)

func (s *Server) handleCreateTemplate(c *gin.Context) {
	var in service.CreateTemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "name is required")
		return
	}
	id, err := s.templates.Create(c.Request.Context(), in, principalID(c))
	if err != nil {
		s.logger.Error("template.create", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (s *Server) handleGetTemplate(c *gin.Context) {
	row, err := s.templates.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, service.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "template not found")
		return
	}
	if err != nil {
		s.logger.Error("template.get", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, domain.TemplateFromGet(row))
}

func (s *Server) handleUpdateTemplate(c *gin.Context) {
	var in service.UpdateTemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := s.templates.Update(c.Request.Context(), c.Param("id"), in, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "template not found")
			return
		}
		s.logger.Error("template.update", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleDeleteTemplate(c *gin.Context) {
	if err := s.templates.SoftDelete(c.Request.Context(), c.Param("id"), principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "template not found")
			return
		}
		s.logger.Error("template.delete", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}
