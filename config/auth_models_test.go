package config

import (
	"testing"
	"time"
)

func TestAuthConfig_GetProfilesForProvider(t *testing.T) {
	cfg := AuthConfig{
		Profiles: map[string]AuthProfileConfig{
			"openai-main":    {Provider: "openai", Mode: AuthModeAPIKey},
			"openai-backup":  {Provider: "openai", Mode: AuthModeAPIKey},
			"anthropic-main": {Provider: "anthropic", Mode: AuthModeAPIKey},
		},
	}

	profiles := cfg.GetProfilesForProvider("openai")
	if len(profiles) != 2 {
		t.Errorf("Expected 2 profiles for openai, got %d", len(profiles))
	}

	profiles = cfg.GetProfilesForProvider("anthropic")
	if len(profiles) != 1 {
		t.Errorf("Expected 1 profile for anthropic, got %d", len(profiles))
	}

	profiles = cfg.GetProfilesForProvider("unknown")
	if len(profiles) != 0 {
		t.Errorf("Expected 0 profiles for unknown, got %d", len(profiles))
	}
}

func TestAuthConfig_GetOrderedProfilesForProvider(t *testing.T) {
	cfg := AuthConfig{
		Profiles: map[string]AuthProfileConfig{
			"openai-main":   {Provider: "openai", Mode: AuthModeAPIKey},
			"openai-backup": {Provider: "openai", Mode: AuthModeAPIKey},
		},
		Order: map[string][]string{
			"openai": {"openai-main", "openai-backup"},
		},
	}

	profiles := cfg.GetOrderedProfilesForProvider("openai")
	if len(profiles) != 2 {
		t.Errorf("Expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0] != "openai-main" || profiles[1] != "openai-backup" {
		t.Errorf("Expected ordered profiles [openai-main, openai-backup], got %v", profiles)
	}
}

func TestAuthConfig_GetCooldown(t *testing.T) {
	tests := []struct {
		name        string
		cfg         AuthConfig
		provider    string
		expectedHrs int
	}{
		{
			name: "default cooldown",
			cfg: AuthConfig{
				Cooldowns: &AuthCooldownConfig{
					BillingBackoffHours: 5,
				},
			},
			provider:    "openai",
			expectedHrs: 5,
		},
		{
			name: "per-provider cooldown",
			cfg: AuthConfig{
				Cooldowns: &AuthCooldownConfig{
					BillingBackoffHours:           5,
					BillingBackoffHoursByProvider: map[string]int{"anthropic": 10},
				},
			},
			provider:    "anthropic",
			expectedHrs: 10,
		},
		{
			name: "capped at max",
			cfg: AuthConfig{
				Cooldowns: &AuthCooldownConfig{
					BillingBackoffHours:           100,
					BillingBackoffHoursByProvider: map[string]int{},
					BillingMaxHours:               24,
				},
			},
			provider:    "openai",
			expectedHrs: 24,
		},
		{
			name:        "nil cooldowns defaults to 5 hours",
			cfg:         AuthConfig{},
			provider:    "openai",
			expectedHrs: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cooldown := tt.cfg.GetCooldown(tt.provider)
			expected := time.Duration(tt.expectedHrs) * time.Hour
			if cooldown != expected {
				t.Errorf("Expected cooldown %v, got %v", expected, cooldown)
			}
		})
	}
}

func TestModelDefinitionConfig_SupportsInput(t *testing.T) {
	model := ModelDefinitionConfig{
		ID:    "gpt-4o",
		Input: []string{"text", "image"},
	}

	if !model.SupportsInput("text") {
		t.Error("Expected model to support text input")
	}
	if !model.SupportsInput("image") {
		t.Error("Expected model to support image input")
	}
	if model.SupportsInput("audio") {
		t.Error("Expected model not to support audio input")
	}
}

func TestModelProviderConfig_GetModel(t *testing.T) {
	provider := ModelProviderConfig{
		BaseURL: "https://api.openai.com/v1",
		Models: []ModelDefinitionConfig{
			{ID: "gpt-4o", Name: "GPT-4o"},
			{ID: "gpt-4-turbo", Name: "GPT-4 Turbo"},
		},
	}

	model := provider.GetModel("gpt-4o")
	if model == nil {
		t.Fatal("Expected model to be found")
	}
	if model.Name != "GPT-4o" {
		t.Errorf("Expected model name 'GPT-4o', got '%s'", model.Name)
	}

	model = provider.GetModel("unknown")
	if model != nil {
		t.Error("Expected nil for unknown model")
	}
}

func TestModelsConfig_GetProvider(t *testing.T) {
	cfg := ModelsConfig{
		Mode: "merge",
		Providers: map[string]ModelProviderConfig{
			"openai": {BaseURL: "https://api.openai.com/v1"},
		},
	}

	provider := cfg.GetProvider("openai")
	if provider == nil {
		t.Fatal("Expected provider to be found")
	}
	if provider.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("Expected base URL 'https://api.openai.com/v1', got '%s'", provider.BaseURL)
	}

	provider = cfg.GetProvider("unknown")
	if provider != nil {
		t.Error("Expected nil for unknown provider")
	}
}

func TestModelsConfig_FindModelByID(t *testing.T) {
	cfg := ModelsConfig{
		Mode: "merge",
		Providers: map[string]ModelProviderConfig{
			"openai": {
				Models: []ModelDefinitionConfig{
					{ID: "gpt-4o", Name: "GPT-4o"},
				},
			},
			"anthropic": {
				Models: []ModelDefinitionConfig{
					{ID: "claude-3-opus", Name: "Claude 3 Opus"},
				},
			},
		},
	}

	providerName, model := cfg.FindModelByID("gpt-4o")
	if providerName != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", providerName)
	}
	if model == nil || model.Name != "GPT-4o" {
		t.Errorf("Expected model 'GPT-4o', got %v", model)
	}

	providerName, model = cfg.FindModelByID("claude-3-opus")
	if providerName != "anthropic" {
		t.Errorf("Expected provider 'anthropic', got '%s'", providerName)
	}

	providerName, model = cfg.FindModelByID("unknown")
	if providerName != "" || model != nil {
		t.Errorf("Expected empty provider and nil model for unknown, got '%s', %v", providerName, model)
	}
}

func TestModelsConfig_HasProviders(t *testing.T) {
	cfg := ModelsConfig{}
	if cfg.HasProviders() {
		t.Error("Expected HasProviders to return false for empty config")
	}

	cfg.Providers = map[string]ModelProviderConfig{
		"openai": {},
	}
	if !cfg.HasProviders() {
		t.Error("Expected HasProviders to return true for non-empty config")
	}
}

func TestDefaultAuthConfig(t *testing.T) {
	cfg := DefaultAuthConfig()
	if cfg.Profiles == nil {
		t.Error("Expected Profiles to be initialized")
	}
	if cfg.Order == nil {
		t.Error("Expected Order to be initialized")
	}
	if cfg.Cooldowns == nil {
		t.Error("Expected Cooldowns to be initialized")
	}
	if cfg.Cooldowns.BillingBackoffHours != 5 {
		t.Errorf("Expected default BillingBackoffHours to be 5, got %d", cfg.Cooldowns.BillingBackoffHours)
	}
}

func TestDefaultModelsConfig(t *testing.T) {
	cfg := DefaultModelsConfig()
	if cfg.Mode != "merge" {
		t.Errorf("Expected default mode 'merge', got '%s'", cfg.Mode)
	}
	if cfg.Providers == nil {
		t.Error("Expected Providers to be initialized")
	}
}
