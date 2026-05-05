//go:build pg

package e2e

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/web-casa/qooim/internal/ai"
	"github.com/web-casa/qooim/tests/testenv"
)

// fakeProvider implements ai.Provider with a hard-coded delta script.
type fakeProvider struct{ deltas []ai.Delta }

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Stream(ctx context.Context, _ ai.ChatRequest, on func(ai.Delta) error) error {
	for _, d := range f.deltas {
		if err := on(d); err != nil {
			return err
		}
	}
	return on(ai.Delta{Done: true})
}

// TestP5AIChatSSE wires a fake provider into the in-process server, then
// verifies POST /api/ai/chat emits the SSE frames in the expected order
// and terminates with [DONE].
func TestP5AIChatSSE(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)
	tok := login(t, s, "admin", "123456")

	// Without a provider the route must 404.
	r := s.POST(t, "/api/ai/chat", "application/json",
		`{"messages":[{"role":"user","content":"hi"}]}`,
		[2]string{"Authorization", "Bearer " + tok})
	mustStatus(t, r, http.StatusNotFound, "ai disabled → 404")

	// Inject a provider and re-test with a streaming request.
	s.SetAIProvider(&fakeProvider{deltas: []ai.Delta{
		{Role: "assistant", Content: "Hello"},
		{Content: " world"},
		{Content: "!"},
	}})

	body := strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)
	req, _ := http.NewRequest(http.MethodPost, s.URL("/api/ai/chat"), body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	var got strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	doneSeen := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			doneSeen = true
			continue
		}
		// Naive content extraction — payload is JSON {"role","content","done"}.
		if i := strings.Index(payload, `"content":"`); i >= 0 {
			rest := payload[i+len(`"content":"`):]
			if j := strings.Index(rest, `"`); j >= 0 {
				got.WriteString(rest[:j])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got.String() != "Hello world!" {
		t.Fatalf("aggregated content = %q, want %q", got.String(), "Hello world!")
	}
	if !doneSeen {
		t.Fatal("expected [DONE] sentinel")
	}

	// Sanity: even when the provider is wired, missing messages must 400.
	r = s.POST(t, "/api/ai/chat", "application/json", `{}`,
		[2]string{"Authorization", "Bearer " + tok})
	mustStatus(t, r, http.StatusBadRequest, "missing messages → 400")
}

// keep-import check: prevent the `httptest` import being elided if I
// drop one of the helpers above.
var _ = httptest.NewServer
