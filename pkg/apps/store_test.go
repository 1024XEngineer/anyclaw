package apps

import (
	"path/filepath"
	"testing"
)

func TestStoreUpsertAndResolveBinding(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	store, err := NewStore(configPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if err := store.Upsert(&Binding{
		App:     "demo-app",
		Name:    "primary",
		Enabled: true,
		Config:  map[string]string{"base_url": "https://example.com"},
		Secrets: map[string]string{"token": "secret"},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	items := store.ListByApp("demo-app")
	if len(items) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(items))
	}

	resolved, err := store.Resolve("demo-app", "primary")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved == nil || resolved.Secrets["token"] != "secret" {
		t.Fatalf("expected resolved binding to include secret")
	}

	envs := ResolveBindingEnvs(resolved)
	if len(envs) == 0 {
		t.Fatal("expected envs for binding")
	}
}

func TestStoreUpsertAndResolvePairing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	store, err := NewStore(configPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if err := store.UpsertPairing(&Pairing{
		App:      "qq-local",
		Workflow: "send-message",
		Name:     "personal-chat",
		Enabled:  true,
		Binding:  "primary",
		Triggers: []string{"给张三发消息", "联系张三"},
		Defaults: map[string]any{"contact": "张三", "human_like": true},
		Metadata: map[string]string{"owner": "local-user"},
	}); err != nil {
		t.Fatalf("UpsertPairing: %v", err)
	}

	items := store.ListPairingsByApp("qq-local")
	if len(items) != 1 {
		t.Fatalf("expected 1 pairing, got %d", len(items))
	}

	resolved, err := store.ResolvePairing("qq-local", "personal-chat")
	if err != nil {
		t.Fatalf("ResolvePairing: %v", err)
	}
	if resolved == nil || resolved.Binding != "primary" {
		t.Fatalf("expected resolved pairing to include binding, got %#v", resolved)
	}
	if resolved.Defaults["contact"] != "张三" {
		t.Fatalf("expected resolved pairing defaults to round-trip, got %#v", resolved.Defaults)
	}
}
