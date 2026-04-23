package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

type Client struct {
	mu         sync.RWMutex
	name       string
	command    string
	args       []string
	env        map[string]string
	transport  string
	tools      []Tool
	resources  []Resource
	prompts    []Prompt
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	running    bool
	reqID      atomic.Int64
	pending    map[int64]chan *Response
	readerDone chan struct{}
	readErr    error
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

type Prompt struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Arguments   []PromptArg `json:"arguments,omitempty"`
}

type PromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func NewClient(name, command string, args []string, env map[string]string) *Client {
	c := &Client{
		name:       name,
		command:    command,
		args:       args,
		env:        env,
		transport:  "stdio",
		pending:    make(map[int64]chan *Response),
		readerDone: make(chan struct{}),
	}
	c.reqID.Store(1)
	return c
}

func (c *Client) Name() string { return c.name }

func (c *Client) Connect(ctx context.Context) error {
	if c.command == "" {
		return fmt.Errorf("no MCP server command configured")
	}

	cmd := exec.CommandContext(ctx, c.command, c.args...)
	if len(c.env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), c.env)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	c.stdin = stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	c.stdout = stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	c.stderr = stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}
	c.cmd = cmd

	c.mu.Lock()
	c.running = true
	c.readErr = nil
	c.pending = make(map[int64]chan *Response)
	c.readerDone = make(chan struct{})
	c.mu.Unlock()

	go c.readResponses()
	go c.readStderr()

	if err := c.initialize(ctx); err != nil {
		c.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	if err := c.discoverTools(ctx); err != nil {
		return fmt.Errorf("discover tools: %w", err)
	}

	c.discoverResources(ctx)
	c.discoverPrompts(ctx)

	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	id := c.nextID()
	req := Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
				"prompts":   map[string]any{},
			},
			"clientInfo": map[string]any{
				"name":    "anyclaw",
				"version": "1.0.0",
			},
		},
	}

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("MCP initialize: %s", resp.Error.Message)
	}

	return c.sendNotification(Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	})
}

func (c *Client) discoverTools(ctx context.Context) error {
	id := c.nextID()
	resp, err := c.sendRequest(ctx, Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "tools/list",
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("tools/list: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil
	}
	toolsRaw, ok := result["tools"].([]any)
	if !ok {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools = make([]Tool, 0, len(toolsRaw))
	for _, t := range toolsRaw {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tm["name"].(string)
		desc, _ := tm["description"].(string)
		schema, _ := tm["inputSchema"].(map[string]any)
		c.tools = append(c.tools, Tool{Name: name, Description: desc, InputSchema: schema})
	}
	return nil
}

func (c *Client) discoverResources(ctx context.Context) {
	id := c.nextID()
	resp, err := c.sendRequest(ctx, Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "resources/list",
	})
	if err != nil {
		return
	}
	if resp.Error != nil {
		return
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return
	}
	resRaw, ok := result["resources"].([]any)
	if !ok {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.resources = make([]Resource, 0, len(resRaw))
	for _, r := range resRaw {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		uri, _ := rm["uri"].(string)
		name, _ := rm["name"].(string)
		desc, _ := rm["description"].(string)
		mime, _ := rm["mimeType"].(string)
		c.resources = append(c.resources, Resource{URI: uri, Name: name, Description: desc, MimeType: mime})
	}
}

func (c *Client) discoverPrompts(ctx context.Context) {
	id := c.nextID()
	resp, err := c.sendRequest(ctx, Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "prompts/list",
	})
	if err != nil {
		return
	}
	if resp.Error != nil {
		return
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return
	}
	pRaw, ok := result["prompts"].([]any)
	if !ok {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = make([]Prompt, 0, len(pRaw))
	for _, p := range pRaw {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		name, _ := pm["name"].(string)
		desc, _ := pm["description"].(string)
		var args []PromptArg
		if argsRaw, ok := pm["arguments"].([]any); ok {
			for _, a := range argsRaw {
				am, ok := a.(map[string]any)
				if !ok {
					continue
				}
				argName, _ := am["name"].(string)
				argDesc, _ := am["description"].(string)
				req, _ := am["required"].(bool)
				args = append(args, PromptArg{Name: argName, Description: argDesc, Required: req})
			}
		}
		c.prompts = append(c.prompts, Prompt{Name: name, Description: desc, Arguments: args})
	}
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	id := c.nextID()
	resp, err := c.sendRequest(ctx, Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tool call error: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func (c *Client) ReadResource(ctx context.Context, uri string) (any, error) {
	id := c.nextID()
	resp, err := c.sendRequest(ctx, Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "resources/read",
		Params: map[string]any{
			"uri": uri,
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("resource read error: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (any, error) {
	id := c.nextID()
	resp, err := c.sendRequest(ctx, Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "prompts/get",
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("prompt get error: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func (c *Client) ListTools() []Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Tool(nil), c.tools...)
}

func (c *Client) ListResources() []Resource {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Resource(nil), c.resources...)
}

func (c *Client) ListPrompts() []Prompt {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Prompt(nil), c.prompts...)
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

func (c *Client) Close() error {
	c.mu.Lock()
	cmd := c.cmd
	stdin := c.stdin
	running := c.running
	c.running = false
	c.mu.Unlock()

	if stdin != nil {
		stdin.Close()
	}
	if cmd != nil && running {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	return nil
}

func (c *Client) sendRequest(ctx context.Context, req Request) (*Response, error) {
	c.mu.RLock()
	if !c.running {
		c.mu.RUnlock()
		return nil, fmt.Errorf("MCP server not running")
	}
	respCh := make(chan *Response, 1)
	if req.ID != nil {
		c.pending[*req.ID] = respCh
	}
	readerDone := c.readerDone
	c.mu.RUnlock()
	defer c.unregisterPending(req.ID)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp := <-respCh:
		if resp == nil {
			return nil, fmt.Errorf("no response")
		}
		return resp, nil
	case <-readerDone:
		return nil, c.connectionReadError()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) sendNotification(req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *Client) readStderr() {
	if c.stderr == nil {
		return
	}
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		fmt.Fprintf(io.Discard, "[mcp:%s] %s\n", c.name, scanner.Text())
	}
}

func (c *Client) readResponses() {
	defer close(c.readerDone)

	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var resp Response
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			c.setReadError(fmt.Errorf("parse response: %w", err))
			return
		}
		if resp.ID == nil {
			continue
		}

		c.mu.RLock()
		ch := c.pending[*resp.ID]
		c.mu.RUnlock()
		if ch != nil {
			ch <- &resp
		}
	}

	if err := scanner.Err(); err != nil {
		c.setReadError(fmt.Errorf("read response: %w", err))
		return
	}
	c.setReadError(fmt.Errorf("no response"))
}

func (c *Client) unregisterPending(id *int64) {
	if id == nil {
		return
	}
	c.mu.Lock()
	delete(c.pending, *id)
	c.mu.Unlock()
}

func (c *Client) setReadError(err error) {
	c.mu.Lock()
	c.readErr = err
	c.running = false
	c.mu.Unlock()
}

func (c *Client) connectionReadError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.readErr != nil {
		return c.readErr
	}
	return fmt.Errorf("MCP server not running")
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}

	merged := append([]string(nil), base...)
	indexes := make(map[string]int, len(merged))
	for i, entry := range merged {
		for j := 0; j < len(entry); j++ {
			if entry[j] == '=' {
				indexes[entry[:j]] = i
				break
			}
		}
	}

	for key, value := range overrides {
		entry := fmt.Sprintf("%s=%s", key, value)
		if idx, ok := indexes[key]; ok {
			merged[idx] = entry
			continue
		}
		indexes[key] = len(merged)
		merged = append(merged, entry)
	}

	return merged
}

func (c *Client) nextID() int64 {
	return c.reqID.Add(1)
}
