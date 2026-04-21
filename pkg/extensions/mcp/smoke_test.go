package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClientConnectAndRegistryUseRealMCPFlow(t *testing.T) {
	t.Setenv("ANYCLAW_TEST_INHERITED", "present")

	client := newHelperClient(map[string]string{
		"MCP_HELPER_EXPECT_INHERITED": "1",
	})
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Fatal("expected connected client")
	}
	if len(client.ListTools()) != 1 || client.ListTools()[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", client.ListTools())
	}
	if len(client.ListResources()) != 1 || client.ListResources()[0].URI != "memo://demo" {
		t.Fatalf("unexpected resources: %#v", client.ListResources())
	}
	if len(client.ListPrompts()) != 1 || client.ListPrompts()[0].Name != "greet" {
		t.Fatalf("unexpected prompts: %#v", client.ListPrompts())
	}

	registry := NewRegistry()
	if err := registry.Register(client.Name(), client); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if got := registry.List(); len(got) != 1 || got[0] != "helper" {
		t.Fatalf("unexpected registry list: %#v", got)
	}

	result, err := registry.CallTool(context.Background(), "helper", "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if resultMap, ok := result.(map[string]any); !ok || len(resultMap["content"].([]any)) != 1 {
		t.Fatalf("unexpected tool result: %#v", result)
	}

	resource, err := registry.ReadResource(context.Background(), "helper", "memo://demo")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if resourceMap, ok := resource.(map[string]any); !ok || len(resourceMap["contents"].([]any)) != 1 {
		t.Fatalf("unexpected resource result: %#v", resource)
	}

	prompt, err := registry.GetPrompt(context.Background(), "helper", "greet", map[string]string{"name": "Ada"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if promptMap, ok := prompt.(map[string]any); !ok || len(promptMap["messages"].([]any)) != 1 {
		t.Fatalf("unexpected prompt result: %#v", prompt)
	}

	allTools := registry.AllTools()
	if len(allTools["helper"]) != 1 {
		t.Fatalf("unexpected all tools: %#v", allTools)
	}
	status := registry.Status()["helper"]
	if !status.Connected || status.Tools != 1 || status.Resources != 1 || status.Prompts != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}

	registry.Remove("helper")
	if client.IsConnected() {
		t.Fatal("expected registry removal to disconnect client")
	}
}

func TestClientConnectClosesProcessWhenToolDiscoveryFails(t *testing.T) {
	t.Parallel()

	client := newHelperClient(map[string]string{
		"MCP_HELPER_FAIL_TOOLS": "1",
	})
	err := client.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "discover tools") {
		t.Fatalf("unexpected connect error: %v", err)
	}
	if client.IsConnected() {
		t.Fatal("expected failed connect to leave client disconnected")
	}
	if client.cmd != nil {
		t.Fatalf("expected failed connect to clean up process, got %#v", client.cmd)
	}
}

func TestServerHandlersReturnStructuredResultsAndErrors(t *testing.T) {
	t.Parallel()

	server := NewServer("anyclaw", "1.0.0")
	server.RegisterTool(ServerTool{
		Name:        "echo",
		Description: "Echo text",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return args["message"], nil
		},
	})
	server.RegisterTool(ServerTool{
		Name:        "fail",
		Description: "Always fail",
		InputSchema: map[string]any{},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return nil, errors.New("boom")
		},
	})
	server.RegisterResource(ServerResource{
		URI:         "memo://demo",
		Name:        "demo",
		Description: "demo resource",
		MimeType:    "text/plain",
		Handler: func(ctx context.Context) (any, error) {
			return "resource", nil
		},
	})
	server.RegisterPrompt(ServerPrompt{
		Name:        "greet",
		Description: "greet prompt",
		Arguments:   []PromptArg{{Name: "name", Required: true}},
		Handler: func(ctx context.Context, args map[string]string) ([]PromptMessage, error) {
			msg := PromptMessage{Role: "assistant"}
			msg.Content.Type = "text"
			msg.Content.Text = "hello " + args["name"]
			return []PromptMessage{msg}, nil
		},
	})

	initResp := server.handleRequest(context.Background(), Request{JSONRPC: "2.0", Method: "initialize"})
	if initResp == nil || initResp.Error != nil {
		t.Fatalf("unexpected initialize response: %#v", initResp)
	}

	toolsResp := server.handleRequest(context.Background(), Request{JSONRPC: "2.0", Method: "tools/list"})
	if toolsResp == nil || toolsResp.Error != nil {
		t.Fatalf("unexpected tools/list response: %#v", toolsResp)
	}

	callResp := server.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  map[string]any{"name": "echo", "arguments": map[string]any{"message": "hello"}},
	})
	if callResp == nil || callResp.Error != nil {
		t.Fatalf("unexpected tools/call response: %#v", callResp)
	}

	callErrResp := server.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  map[string]any{"name": "fail", "arguments": map[string]any{}},
	})
	if callErrResp == nil {
		t.Fatal("expected tools/call error response")
	}
	if resultMap, ok := callErrResp.Result.(map[string]any); !ok || resultMap["isError"] != true {
		t.Fatalf("unexpected error call result: %#v", callErrResp)
	}

	resourceResp := server.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		Method:  "resources/read",
		Params:  map[string]any{"uri": "memo://demo"},
	})
	if resourceResp == nil || resourceResp.Error != nil {
		t.Fatalf("unexpected resource response: %#v", resourceResp)
	}

	promptResp := server.handleRequest(context.Background(), Request{
		JSONRPC: "2.0",
		Method:  "prompts/get",
		Params:  map[string]any{"name": "greet", "arguments": map[string]any{"name": "Ada"}},
	})
	if promptResp == nil || promptResp.Error != nil {
		t.Fatalf("unexpected prompt response: %#v", promptResp)
	}

	notFound := server.handleRequest(context.Background(), Request{JSONRPC: "2.0", Method: "unknown"})
	if notFound == nil || notFound.Error == nil || notFound.Error.Code != -32601 {
		t.Fatalf("unexpected unknown method response: %#v", notFound)
	}
}

func newHelperClient(extraEnv map[string]string) *Client {
	env := map[string]string{
		"GO_WANT_MCP_HELPER": "1",
	}
	for k, v := range extraEnv {
		env[k] = v
	}
	return NewClient("helper", os.Args[0], []string{"-test.run=TestMCPHelperProcess"}, env)
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			if os.Getenv("MCP_HELPER_EXPECT_INHERITED") == "1" && os.Getenv("ANYCLAW_TEST_INHERITED") != "present" {
				writeHelperResponse(Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &Error{Code: -32000, Message: "missing inherited env"},
				})
				continue
			}
			writeHelperResponse(Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "helper", "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			continue
		case "tools/list":
			if os.Getenv("MCP_HELPER_FAIL_TOOLS") == "1" {
				writeHelperResponse(Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &Error{Code: -32000, Message: "tool discovery failed"},
				})
				continue
			}
			writeHelperResponse(Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"tools": []map[string]any{{
						"name":        "echo",
						"description": "Echo text",
						"inputSchema": map[string]any{"type": "object"},
					}},
				},
			})
		case "resources/list":
			writeHelperResponse(Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"resources": []map[string]any{{
						"uri":         "memo://demo",
						"name":        "demo",
						"description": "demo resource",
						"mimeType":    "text/plain",
					}},
				},
			})
		case "prompts/list":
			writeHelperResponse(Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"prompts": []map[string]any{{
						"name":        "greet",
						"description": "greet prompt",
						"arguments": []map[string]any{{
							"name":        "name",
							"description": "person to greet",
							"required":    true,
						}},
					}},
				},
			})
		case "tools/call":
			writeHelperResponse(Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{{"type": "text", "text": "hello"}},
				},
			})
		case "resources/read":
			writeHelperResponse(Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"contents": []map[string]any{{"uri": "memo://demo", "text": "resource"}},
				},
			})
		case "prompts/get":
			writeHelperResponse(Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"messages": []map[string]any{{
						"role": "assistant",
						"content": map[string]any{
							"type": "text",
							"text": "hello Ada",
						},
					}},
				},
			})
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(0)
}

func writeHelperResponse(resp Response) {
	data, _ := json.Marshal(resp)
	_, _ = fmt.Fprintln(os.Stdout, string(data))
}

func TestClientCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	client := newHelperClient(nil)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if client.IsConnected() {
		t.Fatal("expected closed client to report disconnected")
	}
}

func TestRegistryDisconnectAllClosesRegisteredClients(t *testing.T) {
	t.Parallel()

	client := newHelperClient(nil)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	registry := NewRegistry()
	if err := registry.Register("helper", client); err != nil {
		t.Fatalf("Register: %v", err)
	}

	registry.DisconnectAll()
	time.Sleep(20 * time.Millisecond)

	if client.IsConnected() {
		t.Fatal("expected disconnect all to close client")
	}
}
