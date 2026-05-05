package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/domain"
	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
)

func (s *Server) handleCreateRepo(c *gin.Context) {
	var in service.CreateRepoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "name is required")
		return
	}
	id, err := s.repos.Create(c.Request.Context(), in, principalID(c))
	if err != nil {
		s.logger.Error("repo.create", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (s *Server) handleGetRepo(c *gin.Context) {
	row, err := s.repos.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, service.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "repo not found")
		return
	}
	if err != nil {
		s.logger.Error("repo.get", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, domain.RepoFromGet(row))
}

func (s *Server) handleUpdateRepo(c *gin.Context) {
	var in service.UpdateRepoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := s.repos.Update(c.Request.Context(), c.Param("id"), in, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "repo not found")
			return
		}
		s.logger.Error("repo.update", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleDeleteRepo(c *gin.Context) {
	if err := s.repos.Delete(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "repo not found")
			return
		}
		s.logger.Error("repo.delete", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}
