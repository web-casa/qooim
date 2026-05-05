package api

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/httpx"
	"github.com/web-casa/qooim/internal/service"
	"github.com/web-casa/qooim/internal/storage"
)

func (s *Server) handleUploadFile(c *gin.Context) {
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
	res, err := s.files.Upload(c.Request.Context(), service.UploadInput{
		OriginalName: fh.Filename,
		Content:      f,
	}, principalID(c))
	if err != nil {
		s.logger.Error("file.upload", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.JSON(http.StatusCreated, res)
}

func (s *Server) handleDownloadFile(c *gin.Context) {
	rc, row, err := s.files.Open(c.Request.Context(), c.Param("id"))
	if errors.Is(err, service.ErrNotFound) || errors.Is(err, storage.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "not_found", "file not found")
		return
	}
	if err != nil {
		s.logger.Error("file.open", "err", err)
		httpx.Internal(c, "")
		return
	}
	defer rc.Close()
	if row.OriginalName.Valid {
		// mime.FormatMediaType escapes quotes/control chars and encodes
		// non-ASCII via filename* per RFC 5987.
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": row.OriginalName.String,
		}))
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

func (s *Server) handleDeleteFile(c *gin.Context) {
	if err := s.files.SoftDelete(c.Request.Context(), c.Param("id"), principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "not_found", "file not found")
			return
		}
		s.logger.Error("file.delete", "err", err)
		httpx.Internal(c, "")
		return
	}
	c.Status(http.StatusNoContent)
}
