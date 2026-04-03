package llm

import (
	"context"
	"strings"
)

type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
	Name       string
}

type ToolCall struct {
	ID       string
	Type     string
	Function FunctionCall
}

type FunctionCall struct {
	Name string
	Args string
}

type ToolDefinition struct {
	Type     string
	Function FunctionDefinition
}

type FunctionDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Response struct {
	Content      string
	FinishReason string
	Usage        Usage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type ModelCapability struct {
	Name         string
	Chat         bool
	Vision       bool
	FunctionCall bool
	Stream       bool
}

type ClientWrapper struct {
	Provider    string
	Model       string
	APIKey      string
	BaseURL     string
	MaxTokens   int
	Temperature float64
}

func (c *ClientWrapper) SwitchProvider(provider string) error {
	c.Provider = provider
	return nil
}

func (c *ClientWrapper) SwitchModel(model string) error {
	c.Model = model
	return nil
}

func (c *ClientWrapper) SetAPIKey(apiKey string) error {
	c.APIKey = apiKey
	return nil
}

func (c *ClientWrapper) SetTemperature(temp float64) error {
	c.Temperature = temp
	return nil
}

func (c *ClientWrapper) Name() string {
	return c.Provider
}

func (c *ClientWrapper) Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error) {
	return nil, nil
}

func NewClientWrapper(provider, model, apiKey, baseURL string) *ClientWrapper {
	return &ClientWrapper{
		Provider:    provider,
		Model:       model,
		APIKey:      apiKey,
		BaseURL:     baseURL,
		MaxTokens:   4096,
		Temperature: 0.7,
	}
}

func NormalizeProviderName(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.Contains(provider, "qwen") || strings.Contains(provider, "dashscope") || strings.Contains(provider, "alibaba") {
		return "qwen"
	}
	if strings.Contains(provider, "anthropic") || strings.Contains(provider, "claude") {
		return "anthropic"
	}
	if strings.Contains(provider, "ollama") {
		return "ollama"
	}
	if strings.Contains(provider, "compatible") {
		return "compatible"
	}
	return "openai"
}

func ProviderRequiresAPIKey(provider string) bool {
	switch NormalizeProviderName(provider) {
	case "ollama":
		return false
	default:
		return true
	}
}
