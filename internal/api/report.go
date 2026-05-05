package api

import (
	"bytes"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/httpx"
)

func (s *Server) handleProjectReport(c *gin.Context) {
	rep, err := s.reports.Project(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.logger.Error("report.project", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, rep)
}

func (s *Server) handleListExercises(c *gin.Context) {
	out, err := s.reports.Exercises(c.Request.Context())
	if err != nil {
		s.logger.Error("exercises.list", "err", err)
		httpx.Internal(c, "")
		return
	}
	httpx.OK(c, gin.H{"items": out})
}

func (s *Server) handleExportProjectAnswers(c *gin.Context) {
	id := c.Param("id")
	// We can't reliably surface a mid-stream failure once headers fly,
	// so we write the body to an in-memory buffer first. For huge
	// projects we'd want a different transport (chunked + trailers, or
	// a job queue + signed-URL download); P4 keeps it simple and just
	// caps export size via the underlying paginated query.
	var buf bytes.Buffer
	if err := s.reports.ExportProjectAnswers(c.Request.Context(), id, &buf); err != nil {
		s.logger.Error("answers.export", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": "answers-" + id + ".xlsx",
	}))
	_, _ = c.Writer.Write(buf.Bytes())
}

func (s *Server) handleImportTemplates(c *gin.Context) {
	if s.cfg.Storage.MaxUploadBytes > 0 {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.cfg.Storage.MaxUploadBytes)
	}
	fh, err := c.FormFile("file")
	if err != nil {
		httpx.BadRequest(c, "multipart 'file' is required")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.BadRequest(c, "open upload: "+err.Error())
		return
	}
	defer f.Close()
	repoID := c.Param("id")
	created, err := s.reports.ImportTemplatesXLSX(c.Request.Context(), repoID, f, principalID(c), s.q)
	if err != nil {
		s.logger.Error("templates.import", "err", err)
		httpx.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"created": created, "n": strconv.Itoa(created)})
}
