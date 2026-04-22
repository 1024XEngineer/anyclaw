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

func TestSetTemperatureReinitializesClient(t *testing.T) {
	wrapper, err := NewClientWrapper(Config{
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		APIKey:      "test-key",
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatalf("NewClientWrapper: %v", err)
	}

	if err := wrapper.SetTemperature(0.9); err != nil {
		t.Fatalf("SetTemperature: %v", err)
	}

	if wrapper.temperature != 0.9 {
		t.Fatalf("expected wrapper temperature 0.9, got %v", wrapper.temperature)
	}

	inner, ok := wrapper.client.(*client)
	if !ok {
		t.Fatalf("expected inner client type *client, got %T", wrapper.client)
	}
	if inner.temperature != 0.9 {
		t.Fatalf("expected inner client temperature 0.9, got %v", inner.temperature)
	}
}

func TestSetBaseURLReinitializesClient(t *testing.T) {
	wrapper, err := NewClientWrapper(Config{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("NewClientWrapper: %v", err)
	}

	customURL := "https://proxy.example.test/v1"
	if err := wrapper.SetBaseURL(customURL); err != nil {
		t.Fatalf("SetBaseURL: %v", err)
	}

	if wrapper.baseURL != customURL {
		t.Fatalf("expected wrapper baseURL %q, got %q", customURL, wrapper.baseURL)
	}

	inner, ok := wrapper.client.(*client)
	if !ok {
		t.Fatalf("expected inner client type *client, got %T", wrapper.client)
	}
	if inner.baseURL != customURL {
		t.Fatalf("expected inner client baseURL %q, got %q", customURL, inner.baseURL)
	}
}
