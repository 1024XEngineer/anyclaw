package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestMCPClientHelperProcess(t *testing.T) {
	if helperMode() == "" {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := helperResponse(req)
		if resp != nil {
			_ = encoder.Encode(resp)
		}
	}

	os.Exit(0)
}

func TestClientConnectAndCalls(t *testing.T) {
	client := newHelperClient(t, "notify-first")
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Fatal("expected client to be connected")
	}
	if client.Name() != "helper" {
		t.Fatalf("unexpected client name: %q", client.Name())
	}

	if got := client.ListTools(); len(got) != 1 || got[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", got)
	}
	if got := client.ListResources(); len(got) != 1 || got[0].URI != "resource://status" {
		t.Fatalf("unexpected resources: %#v", got)
	}
	if got := client.ListPrompts(); len(got) != 1 || got[0].Name != "review" {
		t.Fatalf("unexpected prompts: %#v", got)
	}

	toolResult, err := client.CallTool(context.Background(), "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if text := firstContentText(toolResult); text != "tool:hi" {
		t.Fatalf("unexpected tool result text: %q", text)
	}

	resourceResult, err := client.ReadResource(context.Background(), "resource://status")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	resourceMap, ok := resourceResult.(map[string]any)
	if !ok {
		t.Fatalf("unexpected resource result type: %T", resourceResult)
	}
	if len(resourceMap["contents"].([]any)) != 1 {
		t.Fatalf("unexpected resource contents: %#v", resourceMap)
	}

	promptResult, err := client.GetPrompt(context.Background(), "review", map[string]string{"focus": "security"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	promptMap, ok := promptResult.(map[string]any)
	if !ok {
		t.Fatalf("unexpected prompt result type: %T", promptResult)
	}
	if len(promptMap["messages"].([]any)) != 1 {
		t.Fatalf("unexpected prompt messages: %#v", promptMap)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if client.IsConnected() {
		t.Fatal("expected client to be disconnected after close")
	}
}

func TestClientConnectPreservesParentEnvironment(t *testing.T) {
	t.Setenv("MCP_TEST_INHERITED", "present")

	client := NewClient("helper", os.Args[0], helperArgs("env-check"), map[string]string{
		"MCP_TEST_CUSTOM": "overlay",
	})
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	result, err := client.CallTool(context.Background(), "echo", map[string]any{"message": "ignored"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if text := firstContentText(result); text != "tool:inherited=present;custom=overlay" {
		t.Fatalf("unexpected env result: %q", text)
	}
}

func TestClientErrors(t *testing.T) {
	t.Run("missing command", func(t *testing.T) {
		client := NewClient("helper", "", nil, nil)
		if err := client.Connect(context.Background()); err == nil || !strings.Contains(err.Error(), "no MCP server command configured") {
			t.Fatalf("expected missing command error, got %v", err)
		}
	})

	t.Run("initialize error", func(t *testing.T) {
		client := newHelperClient(t, "init-error")
		err := client.Connect(context.Background())
		if err == nil || !strings.Contains(err.Error(), "initialize") {
			t.Fatalf("expected initialize error, got %v", err)
		}
	})

	t.Run("call while disconnected", func(t *testing.T) {
		client := NewClient("helper", "unused", nil, nil)
		_, err := client.CallTool(context.Background(), "echo", nil)
		if err == nil || !strings.Contains(err.Error(), "MCP server not running") {
			t.Fatalf("expected disconnected error, got %v", err)
		}
	})
}

func newHelperClient(t *testing.T, mode string) *Client {
	t.Helper()
	return NewClient("helper", os.Args[0], helperArgs(mode), nil)
}

func helperArgs(mode string) []string {
	args := []string{"-test.run=TestMCPClientHelperProcess", "--", "helper"}
	if mode != "" {
		args = append(args, mode)
	}
	return args
}

func helperMode() string {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return strings.Join(os.Args[i+1:], " ")
		}
	}
	return ""
}

func helperResponse(req Request) *Response {
	if helperMode() == "helper init-error" && req.Method == "initialize" {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: -32000, Message: "init failed"},
		}
	}

	switch req.Method {
	case "initialize":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "helper", "version": "1.0.0"},
			},
		}
	case "notifications/initialized":
		fmt.Fprintln(os.Stderr, "initialized")
		return nil
	case "tools/list":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo a message",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			},
		}
	case "resources/list":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"resources": []map[string]any{
					{
						"uri":         "resource://status",
						"name":        "status",
						"description": "Status",
						"mimeType":    "text/plain",
					},
				},
			},
		}
	case "prompts/list":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"prompts": []map[string]any{
					{
						"name":        "review",
						"description": "Review prompt",
						"arguments": []map[string]any{
							{"name": "focus", "description": "Focus", "required": false},
						},
					},
				},
			},
		}
	case "tools/call":
		if helperMode() == "helper notify-first" {
			fmt.Fprintln(os.Stdout, `{"jsonrpc":"2.0","method":"notifications/test","params":{"note":"ready"}}`)
		}
		params := req.Params.(map[string]any)
		args, _ := params["arguments"].(map[string]any)
		message := fmt.Sprintf("%v", args["message"])
		if helperMode() == "helper env-check" {
			message = fmt.Sprintf("inherited=%s;custom=%s", os.Getenv("MCP_TEST_INHERITED"), os.Getenv("MCP_TEST_CUSTOM"))
		}
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "tool:" + message},
				},
			},
		}
	case "resources/read":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"contents": []map[string]any{
					{"uri": "resource://status", "mimeType": "text/plain", "text": "ok"},
				},
			},
		}
	case "prompts/get":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"messages": []map[string]any{
					{
						"role": "user",
						"content": map[string]any{
							"type": "text",
							"text": "review this",
						},
					},
				},
			},
		}
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: -32601, Message: "Method not found"},
		}
	}
}

func TestMergeEnv(t *testing.T) {
	merged := mergeEnv([]string{"A=1", "B=2"}, map[string]string{"B": "updated", "C": "3"})
	got := strings.Join(merged, ",")
	if !strings.Contains(got, "A=1") || !strings.Contains(got, "B=updated") || !strings.Contains(got, "C=3") {
		t.Fatalf("unexpected merged env: %#v", merged)
	}
}

func firstContentText(result any) string {
	resultMap, ok := result.(map[string]any)
	if !ok {
		return ""
	}

	switch content := resultMap["content"].(type) {
	case []any:
		if len(content) == 0 {
			return ""
		}
		first, ok := content[0].(map[string]any)
		if !ok {
			return ""
		}
		text, _ := first["text"].(string)
		return text
	case []map[string]any:
		if len(content) == 0 {
			return ""
		}
		text, _ := content[0]["text"].(string)
		return text
	default:
		return ""
	}
}
