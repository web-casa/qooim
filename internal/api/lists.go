package api

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
)

func parsePage(c *gin.Context) service.Page {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return service.Page{Page: p, PageSize: ps}
}

func (s *Server) handleListProjects(c *gin.Context) {
	res, err := s.listing.ProjectsAsDTO(c.Request.Context(), parsePage(c), service.ProjectFilters{})
	if err != nil {
		s.logger.Error("projects.list", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, res)
}

func (s *Server) handleListRepos(c *gin.Context) {
	res, err := s.listing.Repos(c.Request.Context(), parsePage(c))
	if err != nil {
		s.logger.Error("repos.list", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, res)
}

func (s *Server) handleListTemplates(c *gin.Context) {
	res, err := s.listing.Templates(c.Request.Context(), parsePage(c))
	if err != nil {
		s.logger.Error("templates.list", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, res)
}

func (s *Server) handleListDashboards(c *gin.Context) {
	res, err := s.listing.Dashboards(c.Request.Context(), parsePage(c))
	if err != nil {
		s.logger.Error("dashboards.list", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, res)
}
