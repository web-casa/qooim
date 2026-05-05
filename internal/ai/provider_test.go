package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeProvider returns a canned series of OpenAI-style stream chunks.
func fakeStream(chunks []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, c := range chunks {
			_, _ = io.WriteString(w, "data: "+c+"\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}
}

func TestOpenAICompatible_Stream_OK(t *testing.T) {
	srv := httptest.NewServer(fakeStream([]string{
		`{"choices":[{"delta":{"role":"assistant","content":"Hello"},"finish_reason":""}]}`,
		`{"choices":[{"delta":{"content":" world"},"finish_reason":""}]}`,
		`{"choices":[{"delta":{"content":"!"},"finish_reason":"stop"}]}`,
	}))
	defer srv.Close()

	p := NewOpenAICompatible("test", srv.URL, "test-token", "gpt-x", 5*time.Second)
	var got strings.Builder
	doneSeen := false
	err := p.Stream(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(d Delta) error {
		if d.Done {
			doneSeen = true
			return nil
		}
		got.WriteString(d.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "Hello world!" {
		t.Fatalf("content = %q, want %q", got.String(), "Hello world!")
	}
	if !doneSeen {
		t.Fatalf("expected a Done delta")
	}
}

func TestOpenAICompatible_Stream_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate_limited"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := NewOpenAICompatible("test", srv.URL, "test-token", "gpt-x", 5*time.Second)
	err := p.Stream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(Delta) error { return nil })
	if err == nil {
		t.Fatal("expected error from 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %q, want it to mention 429", err)
	}
}

func TestOpenAICompatible_Stream_NoToken(t *testing.T) {
	p := NewOpenAICompatible("test", "http://example.invalid", "", "m", time.Second)
	err := p.Stream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(Delta) error { return nil })
	if err == nil {
		t.Fatal("expected error without token")
	}
}
