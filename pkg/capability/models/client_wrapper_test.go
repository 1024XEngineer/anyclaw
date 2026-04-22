package llm

import "testing"

func TestNewClientWrapperPreservesConfiguredMaxTokens(t *testing.T) {
	wrapper, err := NewClientWrapper(Config{
		Provider:  "qwen",
		Model:     "qwen-math-turbo",
		APIKey:    "test-key",
		MaxTokens: 3072,
	})
	if err != nil {
		t.Fatalf("NewClientWrapper: %v", err)
	}

	if wrapper.maxTokens != 3072 {
		t.Fatalf("expected maxTokens=3072, got %d", wrapper.maxTokens)
	}
}

func TestSwitchProviderUpdatesDefaultBaseURL(t *testing.T) {
	wrapper, err := NewClientWrapper(Config{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("NewClientWrapper: %v", err)
	}

	if wrapper.baseURL != getDefaultBaseURL("openai") {
		t.Fatalf("expected default openai baseURL, got %q", wrapper.baseURL)
	}

	if err := wrapper.SwitchProvider("anthropic"); err != nil {
		t.Fatalf("SwitchProvider: %v", err)
	}

	if wrapper.provider != "anthropic" {
		t.Fatalf("expected provider anthropic, got %q", wrapper.provider)
	}
	if wrapper.baseURL != getDefaultBaseURL("anthropic") {
		t.Fatalf("expected anthropic default baseURL, got %q", wrapper.baseURL)
	}
}

func TestSwitchProviderPreservesCustomBaseURL(t *testing.T) {
	wrapper, err := NewClientWrapper(Config{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		APIKey:   "test-key",
		BaseURL:  "https://proxy.example.test/v1",
	})
	if err != nil {
		t.Fatalf("NewClientWrapper: %v", err)
	}

	if err := wrapper.SwitchProvider("anthropic"); err != nil {
		t.Fatalf("SwitchProvider: %v", err)
	}

	if wrapper.baseURL != "https://proxy.example.test/v1" {
		t.Fatalf("expected custom baseURL to be preserved, got %q", wrapper.baseURL)
	}
	if wrapper.provider != "anthropic" {
		t.Fatalf("expected provider anthropic, got %q", wrapper.provider)
	}
}
