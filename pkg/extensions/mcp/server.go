package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

type Server struct {
	mu          sync.RWMutex
	name        string
	version     string
	tools       map[string]ServerTool
	resources   map[string]ServerResource
	prompts     map[string]ServerPrompt
	initialized bool
	reqID       atomic.Int64
}

type ServerTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     func(ctx context.Context, args map[string]any) (any, error)
}

type ServerResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
	Handler     func(ctx context.Context) (any, error)
}

type ServerPrompt struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Arguments   []PromptArg `json:"arguments,omitempty"`
	Handler     func(ctx context.Context, args map[string]string) ([]PromptMessage, error)
}

type PromptMessage struct {
	Role    string `json:"role"`
	Content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func NewServer(name, version string) *Server {
	s := &Server{
		name:      name,
		version:   version,
		tools:     make(map[string]ServerTool),
		resources: make(map[string]ServerResource),
		prompts:   make(map[string]ServerPrompt),
	}
	s.reqID.Store(0)
	return s
}

func (s *Server) RegisterTool(tool ServerTool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
}

func (s *Server) RegisterResource(res ServerResource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[res.URI] = res
}

func (s *Server) RegisterPrompt(prompt ServerPrompt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts[prompt.Name] = prompt
}

func (s *Server) ListTools() []ServerTool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]ServerTool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, t)
	}
	return tools
}

func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read error: %w", err)
			}
			return fmt.Errorf("stdin closed")
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := s.handleRequest(ctx, req)
		if resp != nil {
			data, _ := json.Marshal(resp)
			fmt.Println(string(data))
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, req Request) *Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		s.mu.Lock()
		s.initialized = true
		s.mu.Unlock()
		return nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourceRead(ctx, req)
	case "prompts/list":
		return s.handlePromptsList(req)
	case "prompts/get":
		return s.handlePromptGet(ctx, req)
	default:
		id := req.ID
		return &Response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &Error{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

func (s *Server) handleInitialize(req Request) *Response {
	s.mu.Lock()
	s.initialized = false
	s.mu.Unlock()

	id := req.ID
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"resources": map[string]any{"listChanged": true},
				"prompts":   map[string]any{"listChanged": true},
			},
			"serverInfo": map[string]any{
				"name":    s.name,
				"version": s.version,
			},
		},
	}
}

func (s *Server) handleToolsList(req Request) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := req.ID
	tools := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  map[string]any{"tools": tools},
	}
}

func (s *Server) handleToolCall(ctx context.Context, req Request) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := req.ID
	params, ok := req.Params.(map[string]any)
	if !ok {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32602, Message: "Invalid params"}}
	}

	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	tool, ok := s.tools[name]
	if !ok {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32601, Message: "Tool not found: " + name}}
	}

	result, err := tool.Handler(ctx, args)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": fmt.Sprintf("Error: %v", err)},
				},
				"isError": true,
			},
		}
	}

	textResult := fmt.Sprintf("%v", result)
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": textResult},
			},
		},
	}
}

func (s *Server) handleResourcesList(req Request) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := req.ID
	resources := make([]map[string]any, 0, len(s.resources))
	for _, r := range s.resources {
		resources = append(resources, map[string]any{
			"uri":         r.URI,
			"name":        r.Name,
			"description": r.Description,
			"mimeType":    r.MimeType,
		})
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  map[string]any{"resources": resources},
	}
}

func (s *Server) handleResourceRead(ctx context.Context, req Request) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := req.ID
	params, ok := req.Params.(map[string]any)
	if !ok {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32602, Message: "Invalid params"}}
	}

	uri, _ := params["uri"].(string)
	res, ok := s.resources[uri]
	if !ok {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32601, Message: "Resource not found: " + uri}}
	}

	result, err := res.Handler(ctx)
	if err != nil {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32000, Message: err.Error()}}
	}

	textResult := fmt.Sprintf("%v", result)
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"contents": []map[string]any{
				{"uri": uri, "mimeType": res.MimeType, "text": textResult},
			},
		},
	}
}

func (s *Server) handlePromptsList(req Request) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := req.ID
	prompts := make([]map[string]any, 0, len(s.prompts))
	for _, p := range s.prompts {
		prompts = append(prompts, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"arguments":   p.Arguments,
		})
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  map[string]any{"prompts": prompts},
	}
}

func (s *Server) handlePromptGet(ctx context.Context, req Request) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := req.ID
	params, ok := req.Params.(map[string]any)
	if !ok {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32602, Message: "Invalid params"}}
	}

	name, _ := params["name"].(string)
	prompt, ok := s.prompts[name]
	if !ok {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32601, Message: "Prompt not found: " + name}}
	}

	args := make(map[string]string)
	if argsRaw, ok := params["arguments"].(map[string]any); ok {
		for k, v := range argsRaw {
			args[k] = fmt.Sprintf("%v", v)
		}
	}

	messages, err := prompt.Handler(ctx, args)
	if err != nil {
		return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: -32000, Message: err.Error()}}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"messages": messages,
		},
	}
}
