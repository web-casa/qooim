package service

import (
	"context"

	"github.com/web-casa/qooim/internal/ai"
)

// AIService wraps a Provider so handlers don't import the ai package
// directly. It also gives us an injection point for tests (swap in a
// mock provider).
type AIService struct {
	provider ai.Provider
}

func NewAIService(p ai.Provider) *AIService { return &AIService{provider: p} }

// Provider returns the underlying provider name for telemetry.
func (s *AIService) Provider() string {
	if s.provider == nil {
		return ""
	}
	return s.provider.Name()
}

// Chat streams deltas. Returns error if the provider is misconfigured
// or the stream itself fails.
func (s *AIService) Chat(ctx context.Context, req ai.ChatRequest, on func(ai.Delta) error) error {
	if s.provider == nil {
		return ai.ErrDisabled
	}
	return s.provider.Stream(ctx, req, on)
}
