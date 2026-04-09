package config

// ModelAPI defines the API adapter type
type ModelAPI string

const (
	ModelAPIOpenAICompletions ModelAPI = "openai-completions"
	ModelAPIOpenAIResponses   ModelAPI = "openai-responses"
	ModelAPIAnthropicMessages ModelAPI = "anthropic-messages"
	ModelAPIGoogleGenAI       ModelAPI = "google-generative-ai"
	ModelAPIOllama            ModelAPI = "ollama"
)

// ModelCostConfig defines model pricing
type ModelCostConfig struct {
	Input      float64 `mapstructure:"input" json:"input"`             // Cost per million input tokens
	Output     float64 `mapstructure:"output" json:"output"`           // Cost per million output tokens
	CacheRead  float64 `mapstructure:"cache_read" json:"cache_read"`   // Cost per million cached read tokens
	CacheWrite float64 `mapstructure:"cache_write" json:"cache_write"` // Cost per million cache write tokens
}

// ModelCompatConfig defines compatibility settings for OpenAI-compatible APIs
type ModelCompatConfig struct {
	SupportsStore                      bool   `mapstructure:"supports_store" json:"supports_store"`
	SupportsDeveloperRole              bool   `mapstructure:"supports_developer_role" json:"supports_developer_role"`
	SupportsReasoningEffort            bool   `mapstructure:"supports_reasoning_effort" json:"supports_reasoning_effort"`
	SupportsUsageInStreaming           bool   `mapstructure:"supports_usage_in_streaming" json:"supports_usage_in_streaming"`
	SupportsStrictMode                 bool   `mapstructure:"supports_strict_mode" json:"supports_strict_mode"`
	MaxTokensField                     string `mapstructure:"max_tokens_field" json:"max_tokens_field"`
	RequiresToolResultName             bool   `mapstructure:"requires_tool_result_name" json:"requires_tool_result_name"`
	RequiresAssistantAfterToolResult   bool   `mapstructure:"requires_assistant_after_tool_result" json:"requires_assistant_after_tool_result"`
	RequiresThinkingAsText             bool   `mapstructure:"requires_thinking_as_text" json:"requires_thinking_as_text"`
	ThinkingFormat                     string `mapstructure:"thinking_format" json:"thinking_format"`
	SupportsTools                      bool   `mapstructure:"supports_tools" json:"supports_tools"`
	ToolSchemaProfile                  string `mapstructure:"tool_schema_profile" json:"tool_schema_profile"`
	NativeWebSearchTool                bool   `mapstructure:"native_web_search_tool" json:"native_web_search_tool"`
	ToolCallArgumentsEncoding          string `mapstructure:"tool_call_arguments_encoding" json:"tool_call_arguments_encoding"`
	RequiresMistralToolIds             bool   `mapstructure:"requires_mistral_tool_ids" json:"requires_mistral_tool_ids"`
	RequiresOpenAiAnthropicToolPayload bool   `mapstructure:"requires_openai_anthropic_tool_payload" json:"requires_openai_anthropic_tool_payload"`
}

// ModelDefinitionConfig defines a model
type ModelDefinitionConfig struct {
	ID            string             `mapstructure:"id" json:"id"`
	Name          string             `mapstructure:"name" json:"name"`
	API           ModelAPI           `mapstructure:"api" json:"api,omitempty"`
	Reasoning     bool               `mapstructure:"reasoning" json:"reasoning"`
	Input         []string           `mapstructure:"input" json:"input"`
	ContextWindow int                `mapstructure:"contextWindow" json:"contextWindow"`
	MaxTokens     int                `mapstructure:"maxTokens" json:"maxTokens"`
	Cost          *ModelCostConfig   `mapstructure:"cost" json:"cost,omitempty"`
	Headers       map[string]string  `mapstructure:"headers" json:"headers,omitempty"`
	Compat        *ModelCompatConfig `mapstructure:"compat" json:"compat,omitempty"`
}

// SupportsInput checks if the model supports a given input type
func (m *ModelDefinitionConfig) SupportsInput(inputType string) bool {
	for _, input := range m.Input {
		if input == inputType {
			return true
		}
	}
	return false
}

// ModelProviderAuthMode defines the authentication mode for a provider
type ModelProviderAuthMode string

const (
	ModelProviderAuthModeAPIKey ModelProviderAuthMode = "api-key"
	ModelProviderAuthModeAWSSDK ModelProviderAuthMode = "aws-sdk"
	ModelProviderAuthModeOAuth  ModelProviderAuthMode = "oauth"
	ModelProviderAuthModeToken  ModelProviderAuthMode = "token"
)

// ModelProviderConfig defines a provider
type ModelProviderConfig struct {
	BaseURL                     string                  `mapstructure:"baseUrl" json:"baseUrl"`
	APIKey                      string                  `mapstructure:"apiKey" json:"apiKey,omitempty"`
	Auth                        ModelProviderAuthMode   `mapstructure:"auth" json:"auth,omitempty"`
	API                         ModelAPI                `mapstructure:"api" json:"api,omitempty"`
	InjectNumCtxForOpenAICompat bool                    `mapstructure:"inject_num_ctx_for_openai_compat" json:"injectNumCtxForOpenAICompat,omitempty"`
	Headers                     map[string]string       `mapstructure:"headers" json:"headers,omitempty"`
	AuthHeader                  bool                    `mapstructure:"auth_header" json:"authHeader,omitempty"`
	Models                      []ModelDefinitionConfig `mapstructure:"models" json:"models"`
}

// GetModel returns a model by ID
func (p *ModelProviderConfig) GetModel(modelID string) *ModelDefinitionConfig {
	for i := range p.Models {
		if p.Models[i].ID == modelID {
			return &p.Models[i]
		}
	}
	return nil
}

// ModelsConfig defines the models configuration
type ModelsConfig struct {
	Mode      string                         `mapstructure:"mode" json:"mode"` // "merge" or "replace"
	Providers map[string]ModelProviderConfig `mapstructure:"providers" json:"providers"`
}

// DefaultModelsConfig returns default models configuration
func DefaultModelsConfig() ModelsConfig {
	return ModelsConfig{
		Mode:      "merge",
		Providers: make(map[string]ModelProviderConfig),
	}
}

// GetProvider returns a provider by name
func (m *ModelsConfig) GetProvider(providerName string) *ModelProviderConfig {
	if provider, ok := m.Providers[providerName]; ok {
		return &provider
	}
	return nil
}

// GetModel returns a model definition by provider name and model ID
func (m *ModelsConfig) GetModel(providerName, modelID string) *ModelDefinitionConfig {
	provider := m.GetProvider(providerName)
	if provider == nil {
		return nil
	}
	return provider.GetModel(modelID)
}

// FindModelByID searches all providers for a model by ID
func (m *ModelsConfig) FindModelByID(modelID string) (string, *ModelDefinitionConfig) {
	for providerName, provider := range m.Providers {
		if model := provider.GetModel(modelID); model != nil {
			return providerName, model
		}
	}
	return "", nil
}

// HasProviders returns true if any providers are configured
func (m *ModelsConfig) HasProviders() bool {
	return len(m.Providers) > 0
}

// ProviderNames returns a list of all provider names
func (m *ModelsConfig) ProviderNames() []string {
	names := make([]string, 0, len(m.Providers))
	for name := range m.Providers {
		names = append(names, name)
	}
	return names
}
