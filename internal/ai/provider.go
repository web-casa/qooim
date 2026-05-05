// Package ai integrates LLM chat providers behind a thin Provider
// interface. SiliconFlow is the only shipped provider today; any other
// OpenAI-compatible endpoint can plug in by setting Provider="openai" +
// BaseURL.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is a single chat turn (matches OpenAI's role/content shape).
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the input to a Chat call.
type ChatRequest struct {
	Model       string    `json:"model,omitempty"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Delta is one chunk of streamed content as forwarded to the SSE client.
type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Done    bool   `json:"done,omitempty"`
	Err     string `json:"err,omitempty"`
}

// ErrDisabled is returned by services when the AI module is not
// configured (no token / disabled in config) — callers should map it
// to a 404 so the existence of the feature isn't leaked.
var ErrDisabled = errors.New("ai: disabled")

// Provider streams chat completions to the supplied callback. The
// callback returning a non-nil error aborts the stream.
type Provider interface {
	Stream(ctx context.Context, req ChatRequest, on func(Delta) error) error
	Name() string
}

// OpenAICompatible covers SiliconFlow + any OpenAI-equivalent endpoint
// that exposes /v1/chat/completions with stream=true.
type OpenAICompatible struct {
	name    string
	baseURL string
	token   string
	model   string
	http    *http.Client
}

func NewOpenAICompatible(name, baseURL, token, model string, timeout time.Duration) *OpenAICompatible {
	return &OpenAICompatible{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		model:   model,
		http: &http.Client{
			// Streaming responses keep the body open beyond this; the
			// timeout only applies to header receipt.
			Timeout: 0,
			Transport: &http.Transport{
				ResponseHeaderTimeout: timeout,
				IdleConnTimeout:       60 * time.Second,
			},
		},
	}
}

func (p *OpenAICompatible) Name() string { return p.name }

// Stream POSTs to /v1/chat/completions and forwards every "data: ..."
// SSE chunk through the callback. Stream is forced to true.
func (p *OpenAICompatible) Stream(ctx context.Context, req ChatRequest, on func(Delta) error) error {
	if p.token == "" {
		return errors.New("ai: provider token not configured")
	}
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	bodyJSON, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.token)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("provider %d: %s", resp.StatusCode, raw)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return on(Delta{Done: true})
		}
		var chunk struct {
			Choices []struct {
				Delta        Message `json:"delta"`
				FinishReason string  `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// Surface the parse error but keep the stream alive — some
			// providers send keep-alive comments we should skip.
			continue
		}
		for _, ch := range chunk.Choices {
			// Forward content first; emit a separate Done delta only when
			// the provider actually signals end-of-stream so callers can
			// reliably aggregate content without inspecting Done.
			if ch.Delta.Role != "" || ch.Delta.Content != "" {
				if err := on(Delta{Role: ch.Delta.Role, Content: ch.Delta.Content}); err != nil {
					return err
				}
			}
			if ch.FinishReason != "" {
				if err := on(Delta{Done: true}); err != nil {
					return err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}
	// Some servers terminate without explicit [DONE]; signal completion.
	return on(Delta{Done: true})
}
