package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func clearModelsCLIEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"LLM_PROVIDER",
		"LLM_MODEL",
		"LLM_BASE_URL",
		"ANYCLAW_GATEWAY_HOST",
		"ANYCLAW_GATEWAY_BIND",
		"ANYCLAW_GATEWAY_PORT",
		"ANYCLAW_TELEGRAM_BOT_TOKEN",
		"ANYCLAW_TELEGRAM_CHAT_ID",
		"ANYCLAW_SLACK_BOT_TOKEN",
		"ANYCLAW_SLACK_APP_TOKEN",
		"ANYCLAW_SLACK_DEFAULT_CHANNEL",
		"ANYCLAW_DISCORD_BOT_TOKEN",
		"ANYCLAW_DISCORD_DEFAULT_CHANNEL",
		"ANYCLAW_DISCORD_API_BASE_URL",
		"ANYCLAW_DISCORD_GUILD_ID",
		"ANYCLAW_DISCORD_PUBLIC_KEY",
		"ANYCLAW_DISCORD_USE_GATEWAY_WS",
		"ANYCLAW_WHATSAPP_ACCESS_TOKEN",
		"ANYCLAW_WHATSAPP_PHONE_NUMBER_ID",
		"ANYCLAW_WHATSAPP_VERIFY_TOKEN",
		"ANYCLAW_WHATSAPP_APP_SECRET",
		"ANYCLAW_WHATSAPP_DEFAULT_RECIPIENT",
		"ANYCLAW_SIGNAL_BASE_URL",
		"ANYCLAW_SIGNAL_NUMBER",
		"ANYCLAW_SIGNAL_DEFAULT_RECIPIENT",
		"ANYCLAW_SIGNAL_BEARER_TOKEN",
		"ANYCLAW_API_TOKEN",
		"ANYCLAW_WEBHOOK_SECRET",
		"ANYCLAW_RATE_LIMIT_RPM",
		"ANYCLAW_PLUGIN_EXEC_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
}

func TestRunAnyClawCLIRoutesModelsDefaultStatus(t *testing.T) {
	clearModelsCLIEnv(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "qwen"
	cfg.LLM.Model = "qwen-plus"
	if err := cfg.Save(filepath.Join(tempDir, "anyclaw.json")); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"models"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI models: %v", err)
	}
	if !strings.Contains(stdout, "Current model: qwen-plus") || !strings.Contains(stdout, "Provider: qwen") {
		t.Fatalf("unexpected models status output: %q", stdout)
	}
}

func TestRunModelsSetUpdatesDefaultProviderModel(t *testing.T) {
	clearModelsCLIEnv(t)

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

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsCommand([]string{"set", "--config", configPath, "gpt-5"})
	})
	if err != nil {
		t.Fatalf("runModelsCommand set: %v", err)
	}
	if !strings.Contains(stdout, "Default model set to gpt-5") {
		t.Fatalf("unexpected set output: %q", stdout)
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

func TestRunModelsSetUsesEffectiveDefaultProviderProfile(t *testing.T) {
	clearModelsCLIEnv(t)

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
	cfg.LLM.Provider = "qwen"
	cfg.LLM.Model = "qwen-plus"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsSet([]string{"--config", configPath, "gpt-5"})
	})
	if err != nil {
		t.Fatalf("runModelsSet: %v", err)
	}
	if !strings.Contains(stdout, "Default model set to gpt-5") {
		t.Fatalf("unexpected set output: %q", stdout)
	}

	updated, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load updated config: %v", err)
	}
	if updated.LLM.Provider != "openai" {
		t.Fatalf("expected llm.provider to be aligned to default provider profile, got %q", updated.LLM.Provider)
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

func TestRunModelsSetDoesNotPersistEnvOverrides(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	t.Setenv("OPENAI_API_KEY", "sk-secret")
	t.Setenv("LLM_PROVIDER", "anthropic")
	t.Setenv("LLM_MODEL", "claude-sonnet-4-7")
	t.Setenv("LLM_BASE_URL", "https://env.example")
	t.Setenv("ANYCLAW_API_TOKEN", "api-secret")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsSet([]string{"--config", configPath, "gpt-5"})
	})
	if err != nil {
		t.Fatalf("runModelsSet: %v", err)
	}
	if !strings.Contains(stdout, "Default model set to gpt-5") {
		t.Fatalf("unexpected set output: %q", stdout)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile config: %v", err)
	}
	if strings.Contains(string(raw), "sk-secret") || strings.Contains(string(raw), "api-secret") {
		t.Fatalf("expected env secrets to stay out of persisted config, got %s", string(raw))
	}

	var stored config.Config
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("Unmarshal stored config: %v", err)
	}
	if stored.LLM.Provider != "openai" {
		t.Fatalf("expected persisted llm.provider to remain unchanged, got %q", stored.LLM.Provider)
	}
	if stored.LLM.Model != "gpt-5" {
		t.Fatalf("expected persisted llm.model to be updated, got %q", stored.LLM.Model)
	}
	if stored.LLM.APIKey != "" {
		t.Fatalf("expected persisted llm.api_key to stay empty, got %q", stored.LLM.APIKey)
	}
	if stored.LLM.BaseURL != "" {
		t.Fatalf("expected persisted llm.base_url to stay empty, got %q", stored.LLM.BaseURL)
	}
	if stored.Security.APIToken != "" {
		t.Fatalf("expected persisted api_token to stay empty, got %q", stored.Security.APIToken)
	}
}

func TestRunModelsStatusJSON(t *testing.T) {
	clearModelsCLIEnv(t)

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
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsCommand([]string{"status", "--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runModelsCommand status: %v", err)
	}

	var payload struct {
		CurrentProvider string              `json:"current_provider"`
		CurrentModel    string              `json:"current_model"`
		DefaultProvider string              `json:"default_provider"`
		HasDefault      bool                `json:"has_default"`
		Providers       []modelProviderView `json:"providers"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if payload.CurrentProvider != "openai" || payload.CurrentModel != "gpt-4o-mini" {
		t.Fatalf("unexpected current model payload: %#v", payload)
	}
	if payload.DefaultProvider != "openai-main" || !payload.HasDefault {
		t.Fatalf("unexpected default provider payload: %#v", payload)
	}
	if len(payload.Providers) != 1 || payload.Providers[0].Status != "ready" || !payload.Providers[0].HasAPIKey {
		t.Fatalf("unexpected providers payload: %#v", payload.Providers)
	}
}

func TestRunModelsStatusAppliesDefaultProviderProfile(t *testing.T) {
	clearModelsCLIEnv(t)

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
	cfg.LLM.Provider = "qwen"
	cfg.LLM.Model = "qwen-plus"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsStatus([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runModelsStatus: %v", err)
	}
	for _, want := range []string{
		"Current model: gpt-4o-mini",
		"Provider: openai",
		"Default provider: OpenAI Main (openai-main)",
		"runtime=openai model=gpt-4o-mini status=ready",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in status output, got %q", want, stdout)
		}
	}
}

func TestRunModelsStatusTextWithoutProfiles(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Model = "llama3.2"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsStatus([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runModelsStatus: %v", err)
	}
	if !strings.Contains(stdout, "Current model: llama3.2") || !strings.Contains(stdout, "No provider profiles configured") {
		t.Fatalf("unexpected status output: %q", stdout)
	}
}

func TestRunModelsStatusTextWithProfiles(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"
	cfg.LLM.DefaultProviderRef = "openai-main"
	cfg.Providers = []config.ProviderProfile{
		{
			ID:       "openai-main",
			Name:     "OpenAI Main",
			Provider: "openai",
			APIKey:   "sk-test",
			Enabled:  config.BoolPtr(true),
		},
		{
			ID:       "compatible-alt",
			Name:     "Compatible Alt",
			Provider: "compatible",
			APIKey:   "sk-alt",
			BaseURL:  "://bad-url",
			Enabled:  config.BoolPtr(true),
		},
	}

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsStatus([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runModelsStatus: %v", err)
	}
	for _, want := range []string{
		"Default provider: OpenAI Main (openai-main)",
		"Configured providers:",
		"OpenAI Main (openai-main) [default]",
		"runtime=openai model=gpt-4o-mini status=ready",
		"Compatible Alt (compatible-alt)",
		"status=invalid_base_url",
		"note=base URL is invalid",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in status output, got %q", want, stdout)
		}
	}
}

func TestRunModelsListSupportsJSONFilterAndTextCatalog(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "openai-main",
			Name:         "OpenAI Main",
			Provider:     "openai",
			DefaultModel: "gpt-5",
			Enabled:      config.BoolPtr(true),
		},
		{
			ID:           "qwen-main",
			Name:         "Qwen Main",
			Provider:     "qwen",
			DefaultModel: "qwen-plus",
			Enabled:      config.BoolPtr(true),
		},
	}

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsList([]string{"--config", configPath, "--provider", "openai-main", "--json"})
	})
	if err != nil {
		t.Fatalf("runModelsList json: %v", err)
	}
	var filtered struct {
		Provider string   `json:"provider"`
		Models   []string `json:"models"`
	}
	if err := json.Unmarshal([]byte(stdout), &filtered); err != nil {
		t.Fatalf("Unmarshal filtered list: %v\noutput=%s", err, stdout)
	}
	if filtered.Provider != "openai" {
		t.Fatalf("unexpected filtered provider: %#v", filtered)
	}
	if !containsString(filtered.Models, "gpt-5") || !containsString(filtered.Models, "gpt-4o-mini") {
		t.Fatalf("expected merged models in filtered output, got %#v", filtered.Models)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runModelsCommand([]string{"list", "--config", configPath})
	})
	if err != nil {
		t.Fatalf("runModelsCommand list: %v", err)
	}
	if !strings.Contains(stdout, "openai") || !strings.Contains(stdout, "qwen") || !strings.Contains(stdout, "gpt-5") {
		t.Fatalf("unexpected text catalog output: %q", stdout)
	}
}

func TestRunModelsListUsesEffectiveDefaultProvider(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "openai-main",
			Name:         "OpenAI Main",
			Provider:     "openai",
			DefaultModel: "gpt-4o-mini",
			Enabled:      config.BoolPtr(true),
		},
		{
			ID:           "qwen-main",
			Name:         "Qwen Main",
			Provider:     "qwen",
			DefaultModel: "qwen-plus",
			Enabled:      config.BoolPtr(true),
		},
	}
	cfg.LLM.DefaultProviderRef = "openai-main"
	cfg.LLM.Provider = "qwen"
	cfg.LLM.Model = "qwen-plus"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsList([]string{"--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runModelsList: %v", err)
	}

	var payload struct {
		CurrentProvider string              `json:"current_provider"`
		CurrentModel    string              `json:"current_model"`
		Catalog         map[string][]string `json:"catalog"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal models list output: %v\noutput=%s", err, stdout)
	}
	if payload.CurrentProvider != "openai" || payload.CurrentModel != "gpt-4o-mini" {
		t.Fatalf("expected effective provider/model in payload, got %#v", payload)
	}
	if !containsString(payload.Catalog["openai"], "gpt-4o-mini") {
		t.Fatalf("expected effective default model in openai catalog, got %#v", payload.Catalog["openai"])
	}
}

func TestRunModelsCommandUnknownPrintsUsage(t *testing.T) {
	clearModelsCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runModelsCommand([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown models command") {
		t.Fatalf("expected unknown models command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw models commands:") {
		t.Fatalf("expected models usage output, got %q", stdout)
	}
}

func TestRunModelsSetValidatesInput(t *testing.T) {
	clearModelsCLIEnv(t)

	if err := runModelsSet(nil); err == nil || !strings.Contains(err.Error(), "usage: anyclaw models set <model>") {
		t.Fatalf("expected usage error for missing model, got %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := config.DefaultConfig().Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	if err := runModelsSet([]string{"--config", configPath, " "}); err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("expected empty model validation error, got %v", err)
	}
}

func TestModelHelpers(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "openai-main",
			Name:         "OpenAI Main",
			Provider:     "openai",
			DefaultModel: "gpt-5",
			Enabled:      config.BoolPtr(true),
		},
	}

	if got := resolveModelRuntime(cfg, "openai-main"); got != "openai" {
		t.Fatalf("expected provider profile id to resolve runtime, got %q", got)
	}
	if got := resolveModelRuntime(cfg, "qwen"); got != "qwen" {
		t.Fatalf("expected direct runtime filter, got %q", got)
	}
	if got := mergeProviderModels("openai", cfg, []string{"gpt-4o", "gpt-4o-mini", "gpt-4o"}); len(got) != 3 || got[0] != "gpt-4o" {
		t.Fatalf("unexpected merged models: %#v", got)
	}

	if status, message := localProviderHealth(config.ProviderProfile{
		ID:      "disabled",
		Name:    "Disabled Provider",
		Enabled: config.BoolPtr(false),
	}); status != "disabled" || !strings.Contains(message, "disabled") {
		t.Fatalf("expected disabled provider status, got status=%q message=%q", status, message)
	}
	if status, _ := localProviderHealth(config.ProviderProfile{
		ID:      "invalid",
		Name:    "Invalid Provider",
		Enabled: config.BoolPtr(true),
	}); status != "invalid" {
		t.Fatalf("expected invalid provider status, got %q", status)
	}
	if status, _ := localProviderHealth(config.ProviderProfile{
		ID:       "missing-key",
		Name:     "Missing Key",
		Provider: "openai",
		Enabled:  config.BoolPtr(true),
	}); status != "missing_key" {
		t.Fatalf("expected missing_key status, got %q", status)
	}
	if status, _ := localProviderHealth(config.ProviderProfile{
		ID:       "invalid-url",
		Name:     "Invalid URL",
		Provider: "qwen",
		APIKey:   "sk-test",
		BaseURL:  "://bad-url",
		Enabled:  config.BoolPtr(true),
	}); status != "invalid_base_url" {
		t.Fatalf("expected invalid_base_url status, got %q", status)
	}
	if !providerNeedsAPIKey("openai") || providerNeedsAPIKey("ollama") {
		t.Fatalf("unexpected provider key requirements")
	}
	if got := firstNonEmptyModel("gpt-5", "gpt-4o-mini"); got != "gpt-5" {
		t.Fatalf("unexpected firstNonEmptyModel result: %q", got)
	}
	if got := firstNonEmptyModel("", "gpt-4o-mini"); got != "gpt-4o-mini" {
		t.Fatalf("unexpected fallback model result: %q", got)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
