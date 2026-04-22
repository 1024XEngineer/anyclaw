package runtime

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state/policy/secrets"
)

func TestBuildInitialSecretsSnapshotSeedsConfiguredSecrets(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.APIKey = "llm-key"
	cfg.Security.APIToken = "security-token"
	cfg.Channels.Telegram.BotToken = "telegram-token"

	snap := buildInitialSecretsSnapshot(nil, cfg)

	tests := map[string]string{
		"llm_api_key":                "llm-key",
		"security_api_token":         "security-token",
		"channel_telegram_bot_token": "telegram-token",
	}

	for key, want := range tests {
		entry, ok := snap.Get(key)
		if !ok {
			t.Fatalf("expected secret %q in bootstrap snapshot", key)
		}
		if entry.Value != want {
			t.Fatalf("expected secret %q value %q, got %q", key, want, entry.Value)
		}
	}
}

func TestBuildInitialSecretsSnapshotDoesNotPersistSecrets(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.APIKey = "llm-key"

	storeCfg := secrets.DefaultStoreConfig()
	storeCfg.Path = filepath.Join(t.TempDir(), "anyclaw.json")
	store, err := secrets.NewStore(storeCfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	_ = buildInitialSecretsSnapshot(store, cfg)

	if got := store.ListSecrets("", ""); len(got) != 0 {
		t.Fatalf("expected bootstrap snapshot helper to avoid store persistence, got %d stored secrets", len(got))
	}
	if got := store.ListSnapshots(); len(got) != 0 {
		t.Fatalf("expected no persisted snapshots, got %d", len(got))
	}
}

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
