package openai

import (
	"context"

	"anyclaw/pkg/sdk"
)

// Provider is the starter OpenAI-compatible provider implementation.
type Provider struct{}

// New returns a new OpenAI provider instance.
func New() *Provider { return &Provider{} }

// ID returns the provider identifier.
func (p *Provider) ID() string { return "openai" }

// ListModels returns a starter model catalog.
func (p *Provider) ListModels(context.Context) ([]string, error) {
	return []string{"gpt-5.4"}, nil
}

// Stream returns a placeholder stream that can be replaced by a real API client later.
func (p *Provider) Stream(ctx context.Context, req sdk.ModelRequest) (<-chan sdk.ModelChunk, error) {
	out := make(chan sdk.ModelChunk, 2)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case out <- sdk.ModelChunk{Type: "assistant", Text: "OpenAI provider placeholder response for: " + lastMessage(req.Messages), Done: false}:
		}
		out <- sdk.ModelChunk{Type: "assistant", Done: true}
	}()
	return out, nil
}

func lastMessage(messages []sdk.Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}
