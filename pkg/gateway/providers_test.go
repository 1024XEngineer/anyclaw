package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestHandleDefaultProviderSwitchesGlobalDefaultAndBindingResolution(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o-mini"
	cfg.LLM.APIKey = "legacy-key"
	cfg.Agent.WorkDir = filepath.Join(tempDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(tempDir, "workflows", "personal")
	cfg.Security.AuditLog = filepath.Join(tempDir, ".anyclaw", "audit", "audit.jsonl")
	cfg.Skills.Dir = filepath.Join(tempDir, "skills")
	cfg.Plugins.Dir = filepath.Join(tempDir, "plugins")

	enabled := config.BoolPtr(true)
	cfg.Providers = []config.ProviderProfile{
		{
			ID:           "qwen",
			Name:         "Qwen",
			Provider:     "qwen",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIKey:       "provider-key",
			DefaultModel: "qwen-max",
			Enabled:      enabled,
		},
	}
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:            "Go Expert",
			Description:     "Go specialist",
			WorkingDir:      "workflows/go",
			PermissionLevel: "limited",
			Enabled:         enabled,
		},
	}

	configPath := filepath.Join(tempDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	app := &appRuntime.App{
		ConfigPath: configPath,
		Config:     cfg,
		WorkDir:    cfg.Agent.WorkDir,
		WorkingDir: cfg.Agent.WorkingDir,
	}
	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	server := &Server{
		app:   app,
		store: store,
	}

	req := httptest.NewRequest(http.MethodPost, "/providers/default", strings.NewReader(`{"provider_ref":"qwen"}`))
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "operator",
		Permissions: []string{"config.write"},
	}))
	rec := httptest.NewRecorder()

	server.handleDefaultProvider(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if cfg.LLM.DefaultProviderRef != "qwen" {
		t.Fatalf("expected default provider ref qwen, got %q", cfg.LLM.DefaultProviderRef)
	}
	if cfg.LLM.Provider != "qwen" {
		t.Fatalf("expected global provider qwen, got %q", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "qwen-max" {
		t.Fatalf("expected global model qwen-max, got %q", cfg.LLM.Model)
	}
	if cfg.LLM.APIKey != "provider-key" {
		t.Fatalf("expected provider API key to become the global default key")
	}

	var payload providerView
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.IsDefault {
		t.Fatalf("expected response provider to be marked as default")
	}

	binding := server.buildAgentBindingView(cfg.Agent.Profiles[0])
	if !binding.InheritsDefault {
		t.Fatalf("expected blank provider_ref agent to inherit the default provider")
	}
	if binding.ProviderName != "Qwen" {
		t.Fatalf("expected inherited provider name Qwen, got %q", binding.ProviderName)
	}
	if binding.Model != "qwen-max" {
		t.Fatalf("expected inherited model qwen-max, got %q", binding.Model)
	}
}
