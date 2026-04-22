package context

import (
	"reflect"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/state/policy/secrets"
)

func TestResolveSecretUsesStrictTemplateResolution(t *testing.T) {
	snap := testRuntimeSnapshot(map[string]string{
		"llm_api_key": "llm-key",
	})

	got := resolveSecret(snap, "Bearer ${SECRET:llm_api_key}", "llm_api_key")
	if got != "Bearer llm-key" {
		t.Fatalf("expected resolved template, got %q", got)
	}
}

func TestResolveSecretFallsBackToNamedSecretOnMissingReference(t *testing.T) {
	snap := testRuntimeSnapshot(map[string]string{
		"llm_api_key": "llm-key",
	})

	got := resolveSecret(snap, "${SECRET:missing_key}", "llm_api_key")
	if got != "llm-key" {
		t.Fatalf("expected fallback secret value, got %q", got)
	}
}

func TestResolveSecretPreservesPlaintextWhenNothingResolves(t *testing.T) {
	snap := testRuntimeSnapshot(map[string]string{
		"another_key": "another-value",
	})

	got := resolveSecret(snap, "${SECRET:missing_key}", "llm_api_key")
	if got != "${SECRET:missing_key}" {
		t.Fatalf("expected unresolved placeholder to remain unchanged, got %q", got)
	}
}

func TestResolveEmbedderUsesCustomEmbedBaseURL(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.APIKey = "llm-key"
	cfg.LLM.BaseURL = "https://default.example/v1"
	if cfg.LLM.Extra == nil {
		cfg.LLM.Extra = map[string]string{}
	}
	cfg.LLM.Extra["embed_model"] = "text-embedding-3-small"
	cfg.LLM.Extra["embed_base_url"] = "https://embed.example/v1"

	provider := ResolveEmbedder(cfg, nil)
	openAIProvider, ok := provider.(*memory.OpenAIEmbeddingProvider)
	if !ok {
		t.Fatalf("expected OpenAI embedding provider, got %T", provider)
	}
	baseURL := reflect.ValueOf(openAIProvider).Elem().FieldByName("baseURL").String()
	if baseURL != "https://embed.example/v1" {
		t.Fatalf("expected custom embed base URL, got %q", baseURL)
	}
}

func testRuntimeSnapshot(values map[string]string) *secrets.RuntimeSnapshot {
	now := time.Now().UTC()
	entries := make(map[string]*secrets.SecretEntry, len(values))
	for key, value := range values {
		entries[key] = &secrets.SecretEntry{
			ID:        key + "-id",
			Key:       key,
			Value:     value,
			Scope:     secrets.ScopeGlobal,
			Source:    secrets.SourceManual,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	return secrets.NewRuntimeSnapshot(entries, "test")
}
