package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// MCPClient implements the Model Context Protocol client
type MCPClient struct {
	mu       sync.RWMutex
	server   *MCPServer
	tools    []MCPTool
	handlers map[string]MCPToolHandler
}

// MCPServer represents an MCP server process
type MCPServer struct {
	Name      string
	Command   string
	Args      []string
	Env       map[string]string
	Transport string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	running   bool
}

// MCPTool represents a tool exposed by an MCP server
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// MCPToolHandler handles MCP tool calls
type MCPToolHandler func(ctx context.Context, input map[string]any) (any, error)

// MCPRequest represents an MCP JSON-RPC request
type MCPRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// MCPResponse represents an MCP JSON-RPC response
type MCPResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *MCPError `json:"error,omitempty"`
}

// MCPError represents an MCP error
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewMCPClient creates a new MCP client
func NewMCPClient(spec *MCPSpec) *MCPClient {
	if spec == nil {
		return nil
	}

	server := &MCPServer{
		Name:      spec.Name,
		Command:   spec.Command,
		Args:      spec.Args,
		Env:       spec.Env,
		Transport: spec.Transport,
	}

	return &MCPClient{
		server:   server,
		handlers: make(map[string]MCPToolHandler),
	}
}

// Connect starts the MCP server and connects to it
func (c *MCPClient) Connect(ctx context.Context) error {
	if c.server == nil {
		return fmt.Errorf("no MCP server configured")
	}

	cmd := exec.CommandContext(ctx, c.server.Command, c.server.Args...)

	// Set environment variables
	if c.server.Env != nil {
		for k, v := range c.server.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	c.server.stdin = stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	c.server.stdout = stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	c.server.stderr = stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	c.server.cmd = cmd
	c.server.running = true

	// Initialize the connection
	if err := c.initialize(ctx); err != nil {
		c.Close()
		return fmt.Errorf("failed to initialize MCP: %w", err)
	}

	// Discover tools
	if err := c.discoverTools(ctx); err != nil {
		return fmt.Errorf("failed to discover tools: %w", err)
	}

	return nil
}

// initialize sends the initialize request
func (c *MCPClient) initialize(ctx context.Context) error {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
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
		return fmt.Errorf("MCP initialize error: %s", resp.Error.Message)
	}

	// Send initialized notification
	notif := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.sendNotification(notif)
}

// discoverTools discovers available tools from the MCP server
func (c *MCPClient) discoverTools(ctx context.Context) error {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid tools/list response")
	}

	toolsRaw, ok := result["tools"].([]any)
	if !ok {
		return fmt.Errorf("no tools in response")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.tools = make([]MCPTool, 0, len(toolsRaw))
	for _, t := range toolsRaw {
		toolMap, ok := t.(map[string]any)
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		desc, _ := toolMap["description"].(string)
		schema, _ := toolMap["inputSchema"].(map[string]any)

		c.tools = append(c.tools, MCPTool{
			Name:        name,
			Description: desc,
			InputSchema: schema,
		})
	}

	return nil
}

// ListTools returns all discovered tools
func (c *MCPClient) ListTools() []MCPTool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tools
}

// CallTool calls a tool on the MCP server
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      int(time.Now().UnixNano()),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	}

	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("tool call error: %s", resp.Error.Message)
	}

	return resp.Result, nil
}

// sendRequest sends a request and waits for response
func (c *MCPClient) sendRequest(ctx context.Context, req MCPRequest) (*MCPResponse, error) {
	if !c.server.running {
		return nil, fmt.Errorf("MCP server not running")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Write request
	if _, err := c.server.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(c.server.stdout)
	if scanner.Scan() {
		var resp MCPResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &resp, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return nil, fmt.Errorf("no response received")
}

// sendNotification sends a notification (no response expected)
func (c *MCPClient) sendNotification(req MCPRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_, err = c.server.stdin.Write(append(data, '\n'))
	return err
}

// Close closes the MCP connection
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.server.stdin != nil {
		c.server.stdin.Close()
	}

	if c.server.cmd != nil && c.server.running {
		c.server.cmd.Process.Kill()
		c.server.cmd.Wait()
	}

	c.server.running = false
	return nil
}

// IsConnected returns whether the client is connected
func (c *MCPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.server != nil && c.server.running
}

// MCPRegistry manages multiple MCP clients
type MCPRegistry struct {
	mu      sync.RWMutex
	clients map[string]*MCPClient
}

// NewMCPRegistry creates a new MCP registry
func NewMCPRegistry() *MCPRegistry {
	return &MCPRegistry{
		clients: make(map[string]*MCPClient),
	}
}

// Register registers an MCP client
func (r *MCPRegistry) Register(name string, client *MCPClient) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[name]; exists {
		return fmt.Errorf("MCP client already registered: %s", name)
	}

	r.clients[name] = client
	return nil
}

// Get retrieves an MCP client by name
func (r *MCPRegistry) Get(name string) (*MCPClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[name]
	return client, ok
}

// List returns all registered MCP clients
func (r *MCPRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

// ConnectAll connects all registered MCP clients
func (r *MCPRegistry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, client := range r.clients {
		if err := client.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect MCP client %s: %w", name, err)
		}
	}
	return nil
}

// ListAllTools returns all tools from all connected MCP clients
func (r *MCPRegistry) ListAllTools() map[string][]MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]MCPTool)
	for name, client := range r.clients {
		if client.IsConnected() {
			result[name] = client.ListTools()
		}
	}
	return result
}

// CallTool calls a tool on a specific MCP client
func (r *MCPRegistry) CallTool(ctx context.Context, clientName string, toolName string, args map[string]any) (any, error) {
	r.mu.RLock()
	client, ok := r.clients[clientName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("MCP client not found: %s", clientName)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("MCP client not connected: %s", clientName)
	}

	return client.CallTool(ctx, toolName, args)
}

// Close closes all MCP clients
func (r *MCPRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, client := range r.clients {
		client.Close()
	}
	return nil
}
