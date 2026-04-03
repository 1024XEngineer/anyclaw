// Package llm is an alias to providers for backward compatibility.
package llm

import (
	"github.com/anyclaw/anyclaw/pkg/providers"
)

// Re-export types and functions from providers
type ClientWrapper = providers.ClientWrapper
type ProviderConfig = providers.ProviderConfig
type ModelCapability = providers.ModelCapability

func NewClientWrapper(provider, model, apiKey, baseURL string) *ClientWrapper {
	return providers.NewClientWrapper(provider, model, apiKey, baseURL)
}
