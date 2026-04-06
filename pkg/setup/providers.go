package setup

import (
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
)

type ProviderOption struct {
	ID              string
	Label           string
	DefaultModel    string
	AvailableModels []string
	Hint            string
	RequiresAPIKey  bool
}

var providerOptions = []ProviderOption{
	{ID: "openai", Label: "OpenAI", DefaultModel: "gpt-4o-mini", AvailableModels: []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo"}, Hint: "Use your OpenAI API key.", RequiresAPIKey: true},
	{ID: "anthropic", Label: "Anthropic", DefaultModel: "claude-sonnet-4-7", AvailableModels: []string{"claude-opus-4-5", "claude-sonnet-4-7", "claude-haiku-3-5"}, Hint: "Use your Anthropic API key.", RequiresAPIKey: true},
	{ID: "qwen", Label: "Qwen", DefaultModel: "qwen-plus", AvailableModels: []string{"qwen-plus", "qwen-turbo", "qwen-max", "qwen2.5-72b-instruct", "qwen2.5-14b-instruct", "qwq-32b-preview", "qwen-coder-plus"}, Hint: "Use your DashScope API key.", RequiresAPIKey: true},
	{ID: "ollama", Label: "Ollama", DefaultModel: "llama3.2", AvailableModels: []string{"llama3.2", "llama3.1", "codellama", "mistral", "qwen2.5"}, Hint: "No API key needed. Make sure Ollama is running locally.", RequiresAPIKey: false},
	{ID: "compatible", Label: "OpenAI-compatible", DefaultModel: "gpt-4o-mini", AvailableModels: nil, Hint: "Use your compatible endpoint URL and API key.", RequiresAPIKey: true},
}

func ProviderOptions() []ProviderOption {
	items := make([]ProviderOption, len(providerOptions))
	copy(items, providerOptions)
	return items
}

func CanonicalProvider(provider string) string {
	return llm.NormalizeProviderName(provider)
}

func DefaultModelForProvider(provider string) string {
	provider = CanonicalProvider(provider)
	for _, option := range providerOptions {
		if option.ID == provider {
			return option.DefaultModel
		}
	}
	return "gpt-4o-mini"
}

func AvailableModelsForProvider(provider string) []string {
	provider = CanonicalProvider(provider)
	for _, option := range providerOptions {
		if option.ID == provider {
			return option.AvailableModels
		}
	}
	return nil
}

func ProviderLabel(provider string) string {
	provider = CanonicalProvider(provider)
	for _, option := range providerOptions {
		if option.ID == provider {
			return option.Label
		}
	}
	if provider == "" {
		return "Unknown"
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}

func ProviderHint(provider string) string {
	provider = CanonicalProvider(provider)
	for _, option := range providerOptions {
		if option.ID == provider {
			return option.Hint
		}
	}
	return "Provide the model, endpoint, and credentials for your provider."
}

func ProviderNeedsAPIKey(provider string) bool {
	return llm.ProviderRequiresAPIKey(provider)
}

func DefaultBaseURLForProvider(provider string) string {
	provider = CanonicalProvider(provider)
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	default:
		return ""
	}
}

func ResolveProviderChoice(input string, fallback string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return CanonicalProvider(fallback)
	}
	for idx, option := range providerOptions {
		if input == option.ID {
			return option.ID
		}
		if input == fmt.Sprintf("%d", idx+1) {
			return option.ID
		}
	}
	switch input {
	case "alibaba", "dashscope", "tongyi":
		return "qwen"
	case "claude":
		return "anthropic"
	case "local":
		return "ollama"
	default:
		return CanonicalProvider(input)
	}
}

func EnsurePrimaryProviderProfile(cfg *config.Config, provider string, model string, apiKey string, baseURL string) string {
	if cfg == nil {
		return ""
	}
	provider = CanonicalProvider(provider)
	if strings.TrimSpace(model) == "" {
		model = DefaultModelForProvider(provider)
	}

	profileID := "primary-" + provider
	profileName := "Primary " + ProviderLabel(provider)
	profileType := provider
	if provider == "compatible" {
		profileType = "openai-compatible"
	}

	profile := config.ProviderProfile{
		ID:           profileID,
		Name:         profileName,
		Type:         profileType,
		Provider:     provider,
		BaseURL:      strings.TrimSpace(baseURL),
		APIKey:       strings.TrimSpace(apiKey),
		DefaultModel: strings.TrimSpace(model),
		Enabled:      config.BoolPtr(true),
	}
	if existing, ok := cfg.FindProviderProfile(profileID); ok {
		if profile.BaseURL == "" {
			profile.BaseURL = existing.BaseURL
		}
		if profile.APIKey == "" {
			profile.APIKey = existing.APIKey
		}
		if profile.DefaultModel == "" {
			profile.DefaultModel = existing.DefaultModel
		}
		if profile.Type == "" {
			profile.Type = existing.Type
		}
		if len(profile.Capabilities) == 0 {
			profile.Capabilities = append([]string(nil), existing.Capabilities...)
		}
	}

	_ = cfg.UpsertProviderProfile(profile)
	_ = cfg.SetDefaultProviderProfile(profileID)
	cfg.LLM.Provider = provider
	cfg.LLM.Model = firstNonEmpty(strings.TrimSpace(model), cfg.LLM.Model, DefaultModelForProvider(provider))
	if strings.TrimSpace(baseURL) != "" || provider == "compatible" {
		cfg.LLM.BaseURL = strings.TrimSpace(baseURL)
	}
	if strings.TrimSpace(apiKey) != "" || !ProviderNeedsAPIKey(provider) {
		cfg.LLM.APIKey = strings.TrimSpace(apiKey)
	}
	return profileID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
