package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/domain"
	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
)

func principalID(c *gin.Context) string {
	if p, ok := auth.FromContext(c); ok {
		return p.UserID
	}
	return ""
}

func (s *Server) handleCreateProject(c *gin.Context) {
	var in service.CreateProjectInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "name is required")
		return
	}
	id, err := s.projects.Create(c.Request.Context(), in, principalID(c))
	if err != nil {
		s.logger.Error("project.create", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (s *Server) handleGetProject(c *gin.Context) {
	row, err := s.projects.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, service.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "project not found")
		return
	}
	if err != nil {
		s.logger.Error("project.get", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, domain.ProjectFromGet(row))
}

func (s *Server) handleUpdateProject(c *gin.Context) {
	var in service.UpdateProjectInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := s.projects.Update(c.Request.Context(), c.Param("id"), in, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "project not found")
			return
		}
		s.logger.Error("project.update", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleDeleteProject(c *gin.Context) {
	if err := s.projects.SoftDelete(c.Request.Context(), c.Param("id"), principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "project not found")
			return
		}
		s.logger.Error("project.delete", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}
