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
