package reply

import (
	"context"
	"errors"
	"testing"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
)

type stubHook struct {
	messageCalls int
	err          error
}

func (h *stubHook) OnMessage(ctx context.Context, msg *Message) error {
	_, _ = ctx, msg
	h.messageCalls++
	return h.err
}

func (h *stubHook) OnResponse(ctx context.Context, resp *Response) error {
	_, _ = ctx, resp
	return nil
}

type stubLLMClient struct {
	resp *llm.Response
	err  error
}

func (c *stubLLMClient) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	_, _, _ = ctx, messages, tools
	return c.resp, c.err
}

func (c *stubLLMClient) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error {
	_, _, _, _ = ctx, messages, tools, onChunk
	return nil
}

func (c *stubLLMClient) Name() string {
	return "stub"
}

func TestDispatcherCommandAndParsing(t *testing.T) {
	dispatcher := NewDispatcher(tools.NewRegistry())
	hook := &stubHook{}
	dispatcher.RegisterHook(hook)
	dispatcher.RegisterCommand(CommandHandler{
		Name: "echo",
		Handler: func(ctx context.Context, args map[string]string) (string, error) {
			_ = ctx
			if _, ok := args["hello world"]; !ok {
				t.Fatalf("expected parsed arg 'hello world', got %#v", args)
			}
			return "ok", nil
		},
	})

	resp, err := dispatcher.Dispatch(context.Background(), &Message{
		ID:   "msg-1",
		Text: `/echo "hello world"`,
	})
	if err != nil {
		t.Fatalf("Dispatch(command): %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("unexpected response text: %q", resp.Text)
	}
	if hook.messageCalls != 1 {
		t.Fatalf("expected hook message calls = 1, got %d", hook.messageCalls)
	}

	args := splitArgs(`one "two words" three`)
	if len(args) != 3 || args[1] != "two words" {
		t.Fatalf("splitArgs() = %#v, want quoted token preserved", args)
	}
}

func TestDispatcherAgentFallbacks(t *testing.T) {
	dispatcher := NewDispatcher(nil)
	dispatcher.RegisterAgent(&AgentHandler{
		Name:     "assistant",
		Provider: "provider-a",
	})

	resp, err := dispatcher.Dispatch(context.Background(), &Message{ID: "msg-2", Text: "hello"})
	if err != nil {
		t.Fatalf("Dispatch(no provider): %v", err)
	}
	if resp.Text != "Provider not available" {
		t.Fatalf("unexpected missing-provider response: %q", resp.Text)
	}

	dispatcher.RegisterLLM("provider-a", &stubLLMClient{resp: &llm.Response{Content: "agent-reply"}})
	resp, err = dispatcher.Dispatch(context.Background(), &Message{ID: "msg-3", Text: "hello"})
	if err != nil {
		t.Fatalf("Dispatch(agent): %v", err)
	}
	if resp.Text != "agent-reply" {
		t.Fatalf("unexpected agent response: %q", resp.Text)
	}

	dispatcher.RegisterLLM("provider-a", &stubLLMClient{err: errors.New("llm-failed")})
	if _, err := dispatcher.Dispatch(context.Background(), &Message{ID: "msg-4", Text: "hello"}); err == nil {
		t.Fatal("expected llm error to be returned")
	}
}

func TestDispatcherNoAgentAndHookError(t *testing.T) {
	dispatcher := NewDispatcher(nil)

	resp, err := dispatcher.Dispatch(context.Background(), &Message{ID: "msg-5", Text: "hello"})
	if err != nil {
		t.Fatalf("Dispatch(no agent): %v", err)
	}
	if resp.Text != "No agent available" {
		t.Fatalf("unexpected no-agent response: %q", resp.Text)
	}

	dispatcher.RegisterHook(&stubHook{err: errors.New("hook-failed")})
	if _, err := dispatcher.Dispatch(context.Background(), &Message{ID: "msg-6", Text: "hello"}); err == nil {
		t.Fatal("expected hook error to abort dispatch")
	}
}