package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestRunAnyClawCLIRoutesConfigCommand(t *testing.T) {
	clearModelsCLIEnv(t)

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"config", "file", "--config", configPath})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI config: %v", err)
	}
	if strings.TrimSpace(stdout) != config.ResolveConfigPath(configPath) {
		t.Fatalf("unexpected config file output: %q", stdout)
	}
}

func TestCLIUsageIncludesConfigCommand(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI help: %v", err)
	}
	if !strings.Contains(stdout, "anyclaw config <subcommand>") {
		t.Fatalf("expected config help entry, got %q", stdout)
	}
}

func TestRunConfigSetGetAndUnset(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	_, _, err := captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"set", "--config", configPath, "plugins.enabled[0]", "demo-plugin"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand set: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"get", "--config", configPath, "plugins.enabled[0]"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand get: %v", err)
	}
	if strings.TrimSpace(stdout) != "demo-plugin" {
		t.Fatalf("unexpected get output: %q", stdout)
	}

	_, _, err = captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"unset", "--config", configPath, "plugins.enabled[0]"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand unset: %v", err)
	}

	_, _, err = captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"get", "--config", configPath, "plugins.enabled[0]"})
	})
	if err == nil {
		t.Fatal("expected get on removed path to fail")
	}
}

func TestRunConfigGetUsesEffectiveConfigDocument(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	t.Setenv("LLM_PROVIDER", "anthropic")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"get", "--config", configPath, "llm.provider"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand get: %v", err)
	}
	if strings.TrimSpace(stdout) != "anthropic" {
		t.Fatalf("expected effective provider from env override, got %q", stdout)
	}
}

func TestRunConfigSetDoesNotPersistEnvOverrides(t *testing.T) {
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

	_, _, err := captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"set", "--config", configPath, "llm.model", "gpt-5"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand set: %v", err)
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

func TestRunConfigValidateJSON(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"validate", "--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand validate: %v", err)
	}

	var payload struct {
		OK   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true, got %#v", payload)
	}
	if strings.TrimSpace(payload.Path) == "" {
		t.Fatalf("expected path in payload, got %#v", payload)
	}
}

func TestRunConfigValidateUsesPersistedLoadNormalization(t *testing.T) {
	clearModelsCLIEnv(t)

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	data := []byte(`{
  "gateway": {
    "control_ui": {
      "base_path": "console/"
    }
  }
}`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"validate", "--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand validate: %v", err)
	}

	var payload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if !payload.OK {
		t.Fatalf("expected normalized config to validate, got %#v", payload)
	}
}

func TestRunConfigSetUsesPersistedLoadNormalization(t *testing.T) {
	clearModelsCLIEnv(t)

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	data := []byte(`{
  "gateway": {
    "control_ui": {
      "base_path": "console/"
    }
  }
}`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	_, _, err := captureCLIOutput(t, func() error {
		return runConfigCommand([]string{"set", "--config", configPath, "llm.model", "gpt-5"})
	})
	if err != nil {
		t.Fatalf("runConfigCommand set: %v", err)
	}

	loaded, err := config.LoadPersisted(configPath)
	if err != nil {
		t.Fatalf("LoadPersisted config: %v", err)
	}
	if loaded.Gateway.ControlUI.BasePath != "/console" {
		t.Fatalf("expected normalized base path /console, got %q", loaded.Gateway.ControlUI.BasePath)
	}
	if loaded.LLM.Model != "gpt-5" {
		t.Fatalf("expected llm.model to be updated, got %q", loaded.LLM.Model)
	}
}
