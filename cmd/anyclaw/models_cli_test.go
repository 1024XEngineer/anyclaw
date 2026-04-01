package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestRunModelsSetUpdatesDefaultProviderModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "openai-main",
			Name:         "OpenAI Main",
			Provider:     "openai",
			DefaultModel: "gpt-4o-mini",
			Enabled:      config.BoolPtr(true),
		},
	}
	cfg.LLM.DefaultProviderRef = "openai-main"
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	if err := runModelsCommand([]string{"set", "--config", configPath, "gpt-5"}); err != nil {
		t.Fatalf("runModelsCommand set: %v", err)
	}

	updated, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load updated config: %v", err)
	}
	if updated.LLM.Model != "gpt-5" {
		t.Fatalf("expected llm.model to be updated, got %q", updated.LLM.Model)
	}
	provider, ok := updated.FindDefaultProviderProfile()
	if !ok {
		t.Fatalf("expected default provider profile")
	}
	if provider.DefaultModel != "gpt-5" {
		t.Fatalf("expected provider default_model to be updated, got %q", provider.DefaultModel)
	}
}

func TestRunModelsStatusJSON(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "openai-main",
			Name:         "OpenAI Main",
			Provider:     "openai",
			DefaultModel: "gpt-4o-mini",
			APIKey:       "sk-test",
			Enabled:      config.BoolPtr(true),
		},
	}
	cfg.LLM.DefaultProviderRef = "openai-main"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runModelsCommand([]string{"status", "--config", configPath, "--json"}); err != nil {
			t.Fatalf("runModelsCommand status: %v", err)
		}
	})

	var payload struct {
		CurrentProvider string              `json:"current_provider"`
		CurrentModel    string              `json:"current_model"`
		Providers       []modelProviderView `json:"providers"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, output)
	}
	if payload.CurrentProvider != "openai" {
		t.Fatalf("unexpected current provider: %#v", payload)
	}
	if payload.CurrentModel != "gpt-4o-mini" {
		t.Fatalf("unexpected current model: %#v", payload)
	}
	if len(payload.Providers) != 1 || payload.Providers[0].Status != "ready" {
		t.Fatalf("unexpected providers payload: %#v", payload.Providers)
	}
}
