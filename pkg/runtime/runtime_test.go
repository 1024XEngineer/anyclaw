package runtime

import (
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestResolveSubAgentDefinitionInheritsGlobalLLMDefaults(t *testing.T) {
	global := config.LLMConfig{
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		APIKey:      "key",
		BaseURL:     "https://example.com",
		MaxTokens:   4096,
		Temperature: 0.7,
		Proxy:       "http://proxy",
	}

	def := resolveSubAgentDefinition(config.SubAgentConfig{
		Name: "worker",
	}, global)

	if def.LLMProvider != global.Provider || def.LLMModel != global.Model || def.LLMAPIKey != global.APIKey || def.LLMBaseURL != global.BaseURL || def.LLMProxy != global.Proxy {
		t.Fatalf("expected sub-agent definition to inherit global llm settings, got %+v", def)
	}
	if def.LLMMaxTokens == nil || *def.LLMMaxTokens != global.MaxTokens {
		t.Fatalf("expected llm_max_tokens=%d, got %v", global.MaxTokens, def.LLMMaxTokens)
	}
	if def.LLMTemperature == nil || *def.LLMTemperature != global.Temperature {
		t.Fatalf("expected llm_temperature=%v, got %v", global.Temperature, def.LLMTemperature)
	}
}

func TestResolveSubAgentDefinitionPreservesExplicitZeroOverrides(t *testing.T) {
	global := config.LLMConfig{
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	def := resolveSubAgentDefinition(config.SubAgentConfig{
		Name:           "worker",
		LLMMaxTokens:   config.IntPtr(0),
		LLMTemperature: config.Float64Ptr(0),
	}, global)

	if def.LLMMaxTokens == nil || *def.LLMMaxTokens != 0 {
		t.Fatalf("expected explicit llm_max_tokens=0 to be preserved, got %v", def.LLMMaxTokens)
	}
	if def.LLMTemperature == nil || *def.LLMTemperature != 0 {
		t.Fatalf("expected explicit llm_temperature=0 to be preserved, got %v", def.LLMTemperature)
	}
}
