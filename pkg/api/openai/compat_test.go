package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
)

type stubChatRuntime struct {
	response *llm.Response
	err      error
}

func (s *stubChatRuntime) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	return s.response, s.err
}

func (s *stubChatRuntime) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error {
	if s.err != nil {
		return s.err
	}
	if s.response != nil && onChunk != nil && s.response.Content != "" {
		onChunk(s.response.Content)
	}
	return nil
}

func TestHandleChatCompletionsUsesResolvedRuntime(t *testing.T) {
	runtime := &stubChatRuntime{
		response: &llm.Response{
			Content: "hello from anyclaw",
			Usage: llm.Usage{
				InputTokens:  3,
				OutputTokens: 4,
			},
		},
	}

	handler := NewHandler(func(requestedModel string) (chatRuntime, string, error) {
		if requestedModel != "gpt-test" {
			t.Fatalf("expected requested model gpt-test, got %q", requestedModel)
		}
		return runtime, "resolved-model", nil
	}, func() []string { return []string{"resolved-model"} })

	body, err := json.Marshal(map[string]any{
		"model": "gpt-test",
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.HandleChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 response, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp chatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Model != "resolved-model" {
		t.Fatalf("expected resolved model, got %q", resp.Model)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message == nil || resp.Choices[0].Message.Content != "hello from anyclaw" {
		t.Fatalf("unexpected response payload: %#v", resp)
	}
	if resp.Usage.TotalTokens != 7 {
		t.Fatalf("expected total token count 7, got %#v", resp.Usage)
	}
}
