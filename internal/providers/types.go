package providers

import "anyclaw/pkg/sdk"

// Provider is the internal alias for the public provider contract.
type Provider = sdk.Provider

// ProviderRegistry is the internal alias for the public provider registry contract.
type ProviderRegistry = sdk.ProviderRegistry

// ModelRequest is the provider-facing inference request.
type ModelRequest = sdk.ModelRequest

// ModelChunk is one streamed provider chunk.
type ModelChunk = sdk.ModelChunk
