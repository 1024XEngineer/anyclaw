package sdk

import "context"

// Message is the provider-facing normalized chat message shape.
type Message struct {
	Role    string
	Content string
}

// ModelRequest is the provider-facing inference request.
type ModelRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
	Metadata map[string]any
}

// ModelChunk is one streamed chunk from a provider.
type ModelChunk struct {
	Type  string
	Text  string
	Tool  *ToolCall
	Usage map[string]any
	Done  bool
}

// Provider encapsulates a model backend.
type Provider interface {
	ID() string
	ListModels(ctx context.Context) ([]string, error)
	Stream(ctx context.Context, req ModelRequest) (<-chan ModelChunk, error)
}

// ProviderRegistry stores available model providers.
type ProviderRegistry interface {
	Register(provider Provider)
	Get(id string) (Provider, bool)
	List() []Provider
}
