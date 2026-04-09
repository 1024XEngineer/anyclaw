package providers

import (
	"os"
	"testing"

	"anyclaw/config"
)

func TestProviderResolver_ResolveFromModelsConfig(t *testing.T) {
	// Set up environment variable for testing
	os.Setenv("TEST_API_KEY", "test-key-123")
	defer os.Unsetenv("TEST_API_KEY")

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Model: config.ModelSelection{
					Primary: "openai:gpt-4o",
				},
				MaxTokens: 4096,
			},
		},
		Models: config.ModelsConfig{
			Mode: "merge",
			Providers: map[string]config.ModelProviderConfig{
				"openai": {
					BaseURL: "https://api.openai.com/v1",
					APIKey:  "TEST_API_KEY",
					API:     config.ModelAPIOpenAICompletions,
					Models: []config.ModelDefinitionConfig{
						{
							ID:            "gpt-4o",
							Name:          "GPT-4o",
							ContextWindow: 128000,
							MaxTokens:     16384,
							Reasoning:     false,
							Input:         []string{"text", "image"},
						},
					},
				},
			},
		},
	}

	resolver := NewProviderResolver(cfg)
	resolved, err := resolver.Resolve("openai:gpt-4o")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.ProviderName != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", resolved.ProviderName)
	}

	if resolved.ModelID != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", resolved.ModelID)
	}

	if resolved.APIKey != "test-key-123" {
		t.Errorf("Expected API key from env, got '%s'", resolved.APIKey)
	}

	if resolved.API != config.ModelAPIOpenAICompletions {
		t.Errorf("Expected API type 'openai-completions', got '%s'", resolved.API)
	}

	if resolved.Model == nil {
		t.Fatal("Expected model definition to be non-nil")
	}

	if resolved.GetMaxTokens(4096) != 16384 {
		t.Errorf("Expected max tokens 16384, got %d", resolved.GetMaxTokens(4096))
	}

	if !resolved.SupportsImageInput() {
		t.Error("Expected model to support image input")
	}
}

func TestProviderResolver_ResolveWithEnvVarRef(t *testing.T) {
	os.Setenv("MY_OPENAI_KEY", "sk-test-key")
	defer os.Unsetenv("MY_OPENAI_KEY")

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Model: config.ModelSelection{
					Primary: "gpt-4o",
				},
				MaxTokens: 4096,
			},
		},
		Models: config.ModelsConfig{
			Mode: "merge",
			Providers: map[string]config.ModelProviderConfig{
				"openai": {
					BaseURL: "https://api.openai.com/v1",
					APIKey:  "${MY_OPENAI_KEY}",
					Models: []config.ModelDefinitionConfig{
						{ID: "gpt-4o", Name: "GPT-4o"},
					},
				},
			},
		},
	}

	resolver := NewProviderResolver(cfg)
	resolved, err := resolver.Resolve("gpt-4o")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.APIKey != "sk-test-key" {
		t.Errorf("Expected API key 'sk-test-key', got '%s'", resolved.APIKey)
	}
}

func TestProviderResolver_AnthropicProvider(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Model: config.ModelSelection{
					Primary: "claude-3-opus",
				},
				MaxTokens: 4096,
			},
		},
		Models: config.ModelsConfig{
			Mode: "merge",
			Providers: map[string]config.ModelProviderConfig{
				"anthropic": {
					BaseURL: "https://api.anthropic.com/v1",
					APIKey:  "anthropic-key",
					API:     config.ModelAPIAnthropicMessages,
					Models: []config.ModelDefinitionConfig{
						{
							ID:            "claude-3-opus",
							Name:          "Claude 3 Opus",
							ContextWindow: 200000,
							MaxTokens:     4096,
							Reasoning:     true,
						},
					},
				},
			},
		},
	}

	resolver := NewProviderResolver(cfg)
	resolved, err := resolver.Resolve("anthropic:claude-3-opus")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.ProviderName != "anthropic" {
		t.Errorf("Expected provider 'anthropic', got '%s'", resolved.ProviderName)
	}

	if resolved.ModelID != "claude-3-opus" {
		t.Errorf("Expected model 'claude-3-opus', got '%s'", resolved.ModelID)
	}

	if resolved.APIKey != "anthropic-key" {
		t.Errorf("Expected API key 'anthropic-key', got '%s'", resolved.APIKey)
	}

	if resolved.API != config.ModelAPIAnthropicMessages {
		t.Errorf("Expected API type 'anthropic-messages', got '%s'", resolved.API)
	}

	if resolved.Model.MaxTokens != 4096 {
		t.Errorf("Expected model.MaxTokens 4096, got %d", resolved.Model.MaxTokens)
	}

	if !resolved.IsReasoningModel() {
		t.Error("Expected model to be a reasoning model")
	}
}

func TestProviderResolver_CustomProvider(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Model: config.ModelSelection{
					Primary: "custom:model-v1",
				},
				MaxTokens: 4096,
			},
		},
		Models: config.ModelsConfig{
			Mode: "merge",
			Providers: map[string]config.ModelProviderConfig{
				"custom": {
					BaseURL: "https://api.custom.com/v1",
					APIKey:  "custom-key",
					Models: []config.ModelDefinitionConfig{
						{
							ID:            "model-v1",
							Name:          "Custom Model V1",
							ContextWindow: 32000,
							MaxTokens:     2048,
						},
					},
				},
			},
		},
	}

	resolver := NewProviderResolver(cfg)
	resolved, err := resolver.Resolve("custom:model-v1")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.ProviderName != "custom" {
		t.Errorf("Expected provider 'custom', got '%s'", resolved.ProviderName)
	}

	if resolved.ModelID != "model-v1" {
		t.Errorf("Expected model 'model-v1', got '%s'", resolved.ModelID)
	}

	if resolved.BaseURL != "https://api.custom.com/v1" {
		t.Errorf("Expected base URL 'https://api.custom.com/v1', got '%s'", resolved.BaseURL)
	}
}

func TestProviderResolver_SlashModelRef(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Model: config.ModelSelection{
					Primary: "openai/gpt-4o",
				},
				MaxTokens: 4096,
			},
		},
		Models: config.ModelsConfig{
			Mode: "merge",
			Providers: map[string]config.ModelProviderConfig{
				"openai": {
					BaseURL: "https://api.openai.com/v1",
					APIKey:  "openai-key",
					API:     config.ModelAPIOpenAICompletions,
					Models: []config.ModelDefinitionConfig{
						{ID: "gpt-4o", Name: "GPT-4o"},
					},
				},
			},
		},
	}

	resolver := NewProviderResolver(cfg)
	resolved, err := resolver.Resolve("openai/gpt-4o")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.ProviderName != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", resolved.ProviderName)
	}

	if resolved.ModelID != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", resolved.ModelID)
	}

	if resolved.APIKey != "openai-key" {
		t.Errorf("Expected API key 'openai-key', got '%s'", resolved.APIKey)
	}
}

func TestProviderResolver_MissingProvider(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Model: config.ModelSelection{
					Primary: "openai:gpt-4",
				},
				MaxTokens: 4096,
			},
		},
		Models: config.ModelsConfig{
			Mode:      "merge",
			Providers: map[string]config.ModelProviderConfig{},
		},
	}

	resolver := NewProviderResolver(cfg)
	_, err := resolver.Resolve("openai:gpt-4")
	if err == nil {
		t.Fatal("Expected error for missing provider, got nil")
	}
}

func TestParseProviderModel(t *testing.T) {
	tests := []struct {
		input         string
		expectedProv  string
		expectedModel string
	}{
		{"openai:gpt-4o", "openai", "gpt-4o"},
		{"anthropic:claude-3-opus", "anthropic", "claude-3-opus"},
		{"gpt-4o", "", "gpt-4o"},
		{"custom:model-v1", "custom", "model-v1"},
	}

	for _, tt := range tests {
		prov, model := parseProviderModel(tt.input)
		if prov != tt.expectedProv {
			t.Errorf("parseProviderModel(%q): expected provider '%s', got '%s'", tt.input, tt.expectedProv, prov)
		}
		if model != tt.expectedModel {
			t.Errorf("parseProviderModel(%q): expected model '%s', got '%s'", tt.input, tt.expectedModel, model)
		}
	}
}

func TestResolvedProvider_GetMaxTokens(t *testing.T) {
	tests := []struct {
		name        string
		resolved    *ResolvedProvider
		defaultMax  int
		expectedMax int
	}{
		{
			name: "with model definition",
			resolved: &ResolvedProvider{
				Model: &config.ModelDefinitionConfig{MaxTokens: 16384},
			},
			defaultMax:  4096,
			expectedMax: 16384,
		},
		{
			name: "without model definition",
			resolved: &ResolvedProvider{
				Model: nil,
			},
			defaultMax:  4096,
			expectedMax: 4096,
		},
		{
			name: "model definition with zero max tokens",
			resolved: &ResolvedProvider{
				Model: &config.ModelDefinitionConfig{MaxTokens: 0},
			},
			defaultMax:  4096,
			expectedMax: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolved.GetMaxTokens(tt.defaultMax)
			if result != tt.expectedMax {
				t.Errorf("GetMaxTokens(): expected %d, got %d", tt.expectedMax, result)
			}
		})
	}
}

func TestResolvedProvider_GetContextWindow(t *testing.T) {
	tests := []struct {
		name                  string
		resolved              *ResolvedProvider
		expectedContextWindow int
	}{
		{
			name: "with model definition",
			resolved: &ResolvedProvider{
				Model: &config.ModelDefinitionConfig{ContextWindow: 128000},
			},
			expectedContextWindow: 128000,
		},
		{
			name: "without model definition",
			resolved: &ResolvedProvider{
				Model: nil,
			},
			expectedContextWindow: 0,
		},
		{
			name: "model definition with zero context window",
			resolved: &ResolvedProvider{
				Model: &config.ModelDefinitionConfig{ContextWindow: 0},
			},
			expectedContextWindow: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolved.GetContextWindow()
			if result != tt.expectedContextWindow {
				t.Errorf("GetContextWindow(): expected %d, got %d", tt.expectedContextWindow, result)
			}
		})
	}
}
