package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestServerHandleRequestFlow(t *testing.T) {
	srv := NewServer("demo", "1.2.3")
	srv.RegisterTool(ServerTool{
		Name:        "echo",
		Description: "Echo",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			if args["fail"] == true {
				return nil, assertErr("boom")
			}
			return "echo:" + args["message"].(string), nil
		},
	})
	srv.RegisterResource(ServerResource{
		URI:         "resource://status",
		Name:        "status",
		Description: "Status",
		MimeType:    "text/plain",
		Handler: func(_ context.Context) (any, error) {
			return "ready", nil
		},
	})
	srv.RegisterPrompt(ServerPrompt{
		Name:        "review",
		Description: "Review",
		Arguments:   []PromptArg{{Name: "focus", Description: "Focus", Required: false}},
		Handler: func(_ context.Context, args map[string]string) ([]PromptMessage, error) {
			var msg PromptMessage
			msg.Role = "user"
			msg.Content.Type = "text"
			msg.Content.Text = "focus=" + args["focus"]
			return []PromptMessage{msg}, nil
		},
	})

	initResp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(1), Method: "initialize"})
	if initResp == nil || initResp.Error != nil {
		t.Fatalf("unexpected initialize response: %#v", initResp)
	}
	serverInfo := initResp.Result.(map[string]any)["serverInfo"].(map[string]any)
	if serverInfo["name"] != "demo" || serverInfo["version"] != "1.2.3" {
		t.Fatalf("unexpected initialize server info: %#v", serverInfo)
	}

	if resp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", Method: "notifications/initialized"}); resp != nil {
		t.Fatalf("expected nil notification response, got %#v", resp)
	}
	if !srv.initialized {
		t.Fatal("expected server to be initialized")
	}

	toolsResp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(2), Method: "tools/list"})
	if got := len(toolsResp.Result.(map[string]any)["tools"].([]map[string]any)); got != 1 {
		t.Fatalf("expected one tool, got %d", got)
	}

	toolCallResp := srv.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      int64ptr(3),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "echo",
			"arguments": map[string]any{"message": "hi"},
		},
	})
	if text := firstContentText(toolCallResp.Result); text != "echo:hi" {
		t.Fatalf("unexpected tool response text: %q", text)
	}

	toolErrResp := srv.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      int64ptr(4),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "echo",
			"arguments": map[string]any{"fail": true},
		},
	})
	if isErr, _ := toolErrResp.Result.(map[string]any)["isError"].(bool); !isErr {
		t.Fatalf("expected tool error result, got %#v", toolErrResp.Result)
	}

	if resp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(5), Method: "tools/call", Params: "bad"}); resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected invalid params error, got %#v", resp)
	}
	if resp := srv.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      int64ptr(6),
		Method:  "tools/call",
		Params:  map[string]any{"name": "missing"},
	}); resp.Error == nil || !strings.Contains(resp.Error.Message, "Tool not found") {
		t.Fatalf("expected missing tool error, got %#v", resp)
	}

	resourcesResp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(7), Method: "resources/list"})
	if got := len(resourcesResp.Result.(map[string]any)["resources"].([]map[string]any)); got != 1 {
		t.Fatalf("expected one resource, got %d", got)
	}
	resourceReadResp := srv.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      int64ptr(8),
		Method:  "resources/read",
		Params:  map[string]any{"uri": "resource://status"},
	})
	contents := resourceReadResp.Result.(map[string]any)["contents"].([]map[string]any)
	if len(contents) != 1 || contents[0]["text"] != "ready" {
		t.Fatalf("unexpected resource contents: %#v", contents)
	}
	if resp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(9), Method: "resources/read", Params: "bad"}); resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected invalid resource params error, got %#v", resp)
	}

	promptsResp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(10), Method: "prompts/list"})
	if got := len(promptsResp.Result.(map[string]any)["prompts"].([]map[string]any)); got != 1 {
		t.Fatalf("expected one prompt, got %d", got)
	}
	promptGetResp := srv.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		ID:      int64ptr(11),
		Method:  "prompts/get",
		Params:  map[string]any{"name": "review", "arguments": map[string]any{"focus": "security"}},
	})
	messages := promptGetResp.Result.(map[string]any)["messages"].([]PromptMessage)
	if len(messages) != 1 || messages[0].Content.Text != "focus=security" {
		t.Fatalf("unexpected prompt messages: %#v", messages)
	}
	if resp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(12), Method: "prompts/get", Params: "bad"}); resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected invalid prompt params error, got %#v", resp)
	}

	unknownResp := srv.handleRequest(context.Background(), Request{JSONRPC: "2.0", ID: int64ptr(13), Method: "unknown"})
	if unknownResp.Error == nil || unknownResp.Error.Code != -32601 {
		t.Fatalf("expected unknown method error, got %#v", unknownResp)
	}

	if got := srv.ListTools(); len(got) != 1 || got[0].Name != "echo" {
		t.Fatalf("unexpected registered tools: %#v", got)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func int64ptr(v int64) *int64 { return &v }
