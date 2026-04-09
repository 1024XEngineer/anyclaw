package providers

import (
	"fmt"
	"os"
	"strings"

	"anyclaw/config"
)

// ResolvedProvider contains the resolved provider information
type ResolvedProvider struct {
	ProviderName string                        // Provider name (e.g., "openai", "anthropic")
	ModelID      string                        // Model ID without provider prefix
	BaseURL      string                        // API base URL
	APIKey       string                        // API key (resolved from env or plaintext)
	API          config.ModelAPI               // API type
	Model        *config.ModelDefinitionConfig // Model definition (may be nil)
	Headers      map[string]string             // Custom headers
}

// ProviderResolver resolves provider configuration from model strings
type ProviderResolver struct {
	cfg *config.Config
}

// NewProviderResolver creates a new provider resolver
func NewProviderResolver(cfg *config.Config) *ProviderResolver {
	return &ProviderResolver{cfg: cfg}
}

// Resolve resolves a model string to provider configuration
// Model string format: "provider:model-id" or just "model-id"
func (r *ProviderResolver) Resolve(model string) (*ResolvedProvider, error) {
	if !r.cfg.Models.HasProviders() {
		return nil, fmt.Errorf("no providers configured in models.providers. Please configure your providers using OpenClaw-compatible format")
	}

	// Parse provider:model format
	providerName, modelID := parseProviderModel(model)
	if providerName == "" {
		// Try to find the model across all providers
		providerName, _ = r.cfg.Models.FindModelByID(modelID)
		if providerName == "" {
			return nil, fmt.Errorf("model %s not found in any provider", modelID)
		}
	}

	provider := r.cfg.Models.GetProvider(providerName)
	if provider == nil {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}

	// Find the model definition
	var modelDef *config.ModelDefinitionConfig
	if modelID != "" {
		modelDef = provider.GetModel(modelID)
	}

	// Resolve API key
	apiKey := resolveAPIKey(provider.APIKey)

	// Determine API type
	api := provider.API
	if api == "" {
		api = determineAPIFromProvider(providerName, provider)
	}

	return &ResolvedProvider{
		ProviderName: providerName,
		ModelID:      modelID,
		BaseURL:      provider.BaseURL,
		APIKey:       apiKey,
		API:          api,
		Model:        modelDef,
		Headers:      provider.Headers,
	}, nil
}

// parseProviderModel parses "provider:model-id" format
func parseProviderModel(model string) (provider, modelID string) {
	if idx := strings.Index(model, ":"); idx >= 0 {
		return model[:idx], model[idx+1:]
	}
	if idx := strings.Index(model, "/"); idx >= 0 {
		return model[:idx], model[idx+1:]
	}
	return "", model
}

// resolveAPIKey resolves an API key from environment variable or returns plaintext
func resolveAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}

	// Check if it's an environment variable reference
	if strings.HasPrefix(apiKey, "${") && strings.HasSuffix(apiKey, "}") {
		envVar := apiKey[2 : len(apiKey)-1]
		return os.Getenv(envVar)
	}

	// Check if it's just an environment variable name
	if envValue := os.Getenv(apiKey); envValue != "" {
		return envValue
	}

	// Return as plaintext
	return apiKey
}

// determineAPIFromProvider determines the API type from provider name
func determineAPIFromProvider(providerName string, provider *config.ModelProviderConfig) config.ModelAPI {
	// Check if provider has an API type set
	if provider.API != "" {
		return provider.API
	}

	// Determine from provider name
	switch providerName {
	case "anthropic":
		return config.ModelAPIAnthropicMessages
	case "google", "google-vertex", "google-antigravity":
		return config.ModelAPIGoogleGenAI
	case "ollama":
		return config.ModelAPIOllama
	default:
		return config.ModelAPIOpenAICompletions
	}
}

// GetMaxTokens returns the max tokens for a model
func (r *ResolvedProvider) GetMaxTokens(defaultMax int) int {
	if r.Model != nil && r.Model.MaxTokens > 0 {
		return r.Model.MaxTokens
	}
	return defaultMax
}

// GetContextWindow returns the context window for a model
func (r *ResolvedProvider) GetContextWindow() int {
	if r.Model != nil && r.Model.ContextWindow > 0 {
		return r.Model.ContextWindow
	}
	return 0
}

// IsReasoningModel returns true if the model supports reasoning
func (r *ResolvedProvider) IsReasoningModel() bool {
	return r.Model != nil && r.Model.Reasoning
}

// SupportsImageInput returns true if the model supports image input
func (r *ResolvedProvider) SupportsImageInput() bool {
	return r.Model != nil && r.Model.SupportsInput("image")
}
