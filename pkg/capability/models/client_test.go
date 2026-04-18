package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestNewClientAllowsConfigurationPlaceholderWithoutAPIKey(t *testing.T) {
	client, err := NewClient(Config{
		Provider: "compatible",
		Model:    "demo-model",
	})
	if err != nil {
		t.Fatalf("expected placeholder client without API key to succeed: %v", err)
	}
	if client.Name() != "compatible" {
		t.Fatalf("expected compatible client placeholder, got %q", client.Name())
	}

	_, err = client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected placeholder client to reject chat before API key is configured")
	}
	if got := err.Error(); got != "API key is required. Configure a model provider before chatting" {
		t.Fatalf("unexpected placeholder error: %v", err)
	}
}

func TestChatOpenAICompatibleRejectsEmptyMessageContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {"role": "assistant"},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 1,
				"completion_tokens": 1
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "compatible",
		Model:    "demo-model",
		APIKey:   "test-key",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected empty content response to fail")
	}
	if got := err.Error(); got != "empty response content from API; provider/model may be incompatible with chat/completions" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatOpenAICompatibleSupportsContentBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": [
							{"type": "text", "text": "hello"},
							{"type": "text", "text": " world"}
						]
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 1,
				"completion_tokens": 2
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "compatible",
		Model:    "demo-model",
		APIKey:   "test-key",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello world" {
		t.Fatalf("expected merged block text, got %q", resp.Content)
	}
}
