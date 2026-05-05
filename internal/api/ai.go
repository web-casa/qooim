package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/ai"
	"github.com/web-casa/qooim/internal/httpx"
)

type chatRequest struct {
	Model       string       `json:"model,omitempty"`
	Messages    []ai.Message `json:"messages" binding:"required"`
	Temperature float32      `json:"temperature,omitempty"`
}

func (s *Server) handleAIChat(c *gin.Context) {
	if s.aiSvc == nil {
		// Hidden endpoint when AI is disabled.
		httpx.Error(c, http.StatusNotFound, "not_found", "endpoint not available")
		return
	}
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Messages) == 0 {
		httpx.BadRequest(c, "messages is required")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	c.Writer.WriteHeader(http.StatusOK)

	flush := func() {
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		}
	}

	send := func(d ai.Delta) error {
		// Detect client disconnects so the upstream provider call can
		// abort cleanly via context cancellation.
		select {
		case <-c.Request.Context().Done():
			return c.Request.Context().Err()
		default:
		}
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(c.Writer, "data: "); err != nil {
			return err
		}
		if _, err := c.Writer.Write(b); err != nil {
			return err
		}
		if _, err := io.WriteString(c.Writer, "\n\n"); err != nil {
			return err
		}
		flush()
		return nil
	}

	err := s.aiSvc.Chat(c.Request.Context(), ai.ChatRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
	}, send)

	if err != nil {
		// Headers are already on the wire — we can't switch to 404/500.
		// Send a terminal error frame so the client knows what happened
		// and log the detail server-side. ai.ErrDisabled is filtered out
		// at the top of the handler before headers fly, so reaching it
		// here would be a programming error rather than a runtime case.
		_ = send(ai.Delta{Err: err.Error(), Done: true})
		s.logger.Error("ai.chat", "err", err)
		return
	}
	// Final sentinel for clients that want a clean termination signal.
	_, _ = io.WriteString(c.Writer, "data: [DONE]\n\n")
	flush()
}
