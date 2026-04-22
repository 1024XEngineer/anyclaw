package memory

import "testing"

func TestNewOpenAIEmbeddingProviderUsesCustomBaseURL(t *testing.T) {
	provider := NewOpenAIEmbeddingProvider("key", "text-embedding-3-small", "https://example.com/v1/")

	if provider.baseURL != "https://example.com/v1" {
		t.Fatalf("expected trimmed custom base URL, got %q", provider.baseURL)
	}
}

func TestNewOpenAIEmbeddingProviderFallsBackToDefaultBaseURL(t *testing.T) {
	provider := NewOpenAIEmbeddingProvider("key", "", "")

	if provider.baseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected default base URL, got %q", provider.baseURL)
	}
	if provider.model != "text-embedding-3-small" {
		t.Fatalf("expected default model, got %q", provider.model)
	}
}
