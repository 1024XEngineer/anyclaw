package mock

import (
	"context"

	"anyclaw/pkg/sdk"
)

// Provider is a deterministic mock provider for tests and local bring-up.
type Provider struct{}

// New returns a new mock provider.
func New() *Provider { return &Provider{} }

// ID returns the provider identifier.
func (p *Provider) ID() string { return "mock" }

// ListModels returns the mock model list.
func (p *Provider) ListModels(context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

// Stream emits a fixed mock response.
func (p *Provider) Stream(ctx context.Context, _ sdk.ModelRequest) (<-chan sdk.ModelChunk, error) {
	out := make(chan sdk.ModelChunk, 2)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case out <- sdk.ModelChunk{Type: "assistant", Text: "mock-response", Done: false}:
		}
		out <- sdk.ModelChunk{Type: "assistant", Done: true}
	}()
	return out, nil
}
