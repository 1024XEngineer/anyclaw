package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidateMissingProvider(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Provider = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	if !strings.Contains(err.Error(), "llm.provider") {
		t.Fatalf("error should mention llm.provider: %v", err)
	}
}

func TestValidateMissingModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Model = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "llm.model") {
		t.Fatalf("error should mention llm.model: %v", err)
	}
}

func TestValidateTemperature(t *testing.T) {
	tests := []struct {
		temp  float64
		valid bool
	}{
		{0.0, true},
		{0.7, true},
		{2.0, true},
		{-0.1, false},
		{2.1, false},
	}
	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.LLM.Temperature = tt.temp
		err := cfg.Validate()
		if tt.valid && err != nil {
			t.Errorf("temperature %f should be valid, got error: %v", tt.temp, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("temperature %f should be invalid", tt.temp)
		}
	}
}

func TestValidatePermissionLevel(t *testing.T) {
	validLevels := []string{"full", "limited", "read-only"}
	for _, level := range validLevels {
		cfg := DefaultConfig()
		cfg.Agent.PermissionLevel = level
		if err := cfg.Validate(); err != nil {
			t.Errorf("permission level %q should be valid: %v", level, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Agent.PermissionLevel = "invalid"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid permission level")
	}
	if !strings.Contains(err.Error(), "agent.permission_level") {
		t.Fatalf("error should mention agent.permission_level: %v", err)
	}
}

func TestValidateGatewayPort(t *testing.T) {
	tests := []struct {
		port  int
		valid bool
	}{
		{0, true},
		{8080, true},
		{65535, true},
		{-1, false},
		{65536, false},
	}
	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.Gateway.Port = tt.port
		err := cfg.Validate()
		if tt.valid && err != nil {
			t.Errorf("port %d should be valid, got error: %v", tt.port, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("port %d should be invalid", tt.port)
		}
	}
}

func TestValidateMemoryFormat(t *testing.T) {
	validFormats := []string{"markdown", "json", "txt"}
	for _, format := range validFormats {
		cfg := DefaultConfig()
		cfg.Memory.Format = format
		if err := cfg.Validate(); err != nil {
			t.Errorf("memory format %q should be valid: %v", format, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Memory.Format = "xml"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid memory format")
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Provider = ""
	cfg.LLM.Model = ""
	cfg.Gateway.Port = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for multiple invalid fields")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "llm.provider") {
		t.Error("error should mention llm.provider")
	}
	if !strings.Contains(errStr, "llm.model") {
		t.Error("error should mention llm.model")
	}
	if !strings.Contains(errStr, "gateway.port") {
		t.Error("error should mention gateway.port")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("loading non-existent file should use defaults: %v", err)
	}
	if cfg.LLM.Provider != DefaultConfig().LLM.Provider {
		t.Error("should use default provider")
	}
}

func TestLoadValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.LLM.Provider = "qwen"
	cfg.LLM.Model = "qwen-plus"
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("loading valid config should succeed: %v", err)
	}
	if loaded.LLM.Provider != "qwen" {
		t.Errorf("expected provider qwen, got %s", loaded.LLM.Provider)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("{invalid json}"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse config file") {
		t.Fatalf("error should mention parse failure: %v", err)
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")

	cfg := DefaultConfig()
	cfg.LLM.Provider = ""
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid config values")
	}
	if !strings.Contains(err.Error(), "llm.provider") {
		t.Fatalf("error should mention validation issue: %v", err)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key-123")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("loading config with env override should succeed: %v", err)
	}
	if loaded.LLM.APIKey != "test-key-123" {
		t.Errorf("expected API key from env, got %s", loaded.LLM.APIKey)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.LLM.Provider = "anthropic"
	cfg.Gateway.Port = 9999

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save should succeed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload should succeed: %v", err)
	}
	if loaded.LLM.Provider != "anthropic" {
		t.Errorf("expected provider anthropic, got %s", loaded.LLM.Provider)
	}
	if loaded.Gateway.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loaded.Gateway.Port)
	}
}

func TestSandboxBackendValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sandbox.Backend = "kubernetes"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid sandbox backend")
	}
	if !strings.Contains(err.Error(), "sandbox.backend") {
		t.Fatalf("error should mention sandbox.backend: %v", err)
	}
}

func TestGatewayBindValidation(t *testing.T) {
	validBinds := []string{"", "loopback", "all", "127.0.0.1", "0.0.0.0", "::1"}
	for _, bind := range validBinds {
		cfg := DefaultConfig()
		cfg.Gateway.Bind = bind
		if err := cfg.Validate(); err != nil {
			t.Errorf("gateway.bind %q should be valid: %v", bind, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Gateway.Bind = "invalid-value"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid gateway.bind")
	}
	if !strings.Contains(err.Error(), "gateway.bind") {
		t.Fatalf("error should mention gateway.bind: %v", err)
	}
}
