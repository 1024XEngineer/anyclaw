package llm

import (
	"context"
	"net/url"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestChatAnthropicUsesConfiguredBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("expected anthropic request path /messages, got %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("expected anthropic API key header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{"type": "text", "text": "anthropic ok"}
			],
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 3,
				"output_tokens": 5
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
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
	if resp.Content != "anthropic ok" {
		t.Fatalf("expected anthropic text response, got %q", resp.Content)
	}
}

func TestStreamAnthropicUsesConfiguredBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("expected anthropic request path /messages, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"type":"content_block_delta","content_block_delta":{"type":"text_delta","text":"hello"}}`,
			``,
			`data: {"type":"content_block_delta","content_block_delta":{"type":"text_delta","text":" world"}}`,
			``,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet",
		APIKey:   "test-key",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	var chunks []string
	err = client.StreamChat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, func(chunk string) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if strings.Join(chunks, "") != "hello world" {
		t.Fatalf("expected streamed anthropic content, got %q", strings.Join(chunks, ""))
	}
}

func TestNewHTTPClientUsesSystemProxyFromEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:18080")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:18443")
	t.Setenv("NO_PROXY", "")

	httpClient := newHTTPClient("system")
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", httpClient.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("expected proxy func to be set for system proxy mode")
	}

	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:18080" {
		t.Fatalf("expected system proxy to be used, got %#v", proxyURL)
	}
}

func TestNewHTTPClientUsesExplicitProxyURL(t *testing.T) {
	_ = os.Setenv("HTTP_PROXY", "http://127.0.0.1:18080")
	httpClient := newHTTPClient("http://proxy.example.test:8080")
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", httpClient.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("expected proxy func to be set for explicit proxy mode")
	}

	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	want, _ := url.Parse("http://proxy.example.test:8080")
	if proxyURL == nil || proxyURL.String() != want.String() {
		t.Fatalf("expected explicit proxy %q, got %#v", want.String(), proxyURL)
	}
}
