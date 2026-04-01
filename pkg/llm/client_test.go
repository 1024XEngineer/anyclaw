package llm

import "testing"

func TestProviderRequiresAPIKey(t *testing.T) {
	if ProviderRequiresAPIKey("ollama") {
		t.Fatal("expected ollama to work without an API key")
	}
	if !ProviderRequiresAPIKey("openai") {
		t.Fatal("expected openai to require an API key")
	}
}

func TestNewClientAllowsOllamaWithoutAPIKey(t *testing.T) {
	client, err := NewClient(Config{
		Provider: "ollama",
		Model:    "llama3.2",
	})
	if err != nil {
		t.Fatalf("expected ollama client without API key to succeed: %v", err)
	}
	if client.Name() != "ollama" {
		t.Fatalf("expected ollama client, got %q", client.Name())
	}
}
