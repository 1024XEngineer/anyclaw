package agent

import (
	"context"

	"anyclaw/internal/providers"
	"anyclaw/pkg/sdk"
)

// ModelExecutor isolates provider lookup and streaming inference calls.
type ModelExecutor struct {
	Providers *providers.Registry
}

// Stream resolves a provider and starts streaming chunks.
func (m ModelExecutor) Stream(ctx context.Context, providerID string, req sdk.ModelRequest) (<-chan sdk.ModelChunk, error) {
	provider, ok := m.Providers.Get(providerID)
	if !ok {
		closed := make(chan sdk.ModelChunk)
		close(closed)
		return closed, nil
	}
	return provider.Stream(ctx, req)
}
