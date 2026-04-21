package mcp

import (
	"context"
	"fmt"
	"sync"
)

type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewRegistry() *Registry {
	return &Registry{
		clients: make(map[string]*Client),
	}
}

func (r *Registry) Register(name string, client *Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.clients[name]; exists {
		return fmt.Errorf("MCP client already registered: %s", name)
	}
	r.clients[name] = client
	return nil
}

func (r *Registry) Get(name string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[name]
	return c, ok
}

func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.clients[name]; ok {
		c.Close()
		delete(r.clients, name)
	}
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

func (r *Registry) ConnectAll(ctx context.Context) []error {
	r.mu.RLock()
	clients := make(map[string]*Client)
	for name, c := range r.clients {
		clients[name] = c
	}
	r.mu.RUnlock()

	var errs []error
	for name, c := range clients {
		if err := c.Connect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("MCP %s: %w", name, err))
		}
	}
	return errs
}

func (r *Registry) DisconnectAll() {
	r.mu.RLock()
	clients := make([]*Client, 0, len(r.clients))
	for _, c := range r.clients {
		clients = append(clients, c)
	}
	r.mu.RUnlock()

	for _, c := range clients {
		c.Close()
	}
}

func (r *Registry) AllTools() map[string][]Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string][]Tool)
	for name, c := range r.clients {
		if c.IsConnected() {
			result[name] = c.ListTools()
		}
	}
	return result
}

func (r *Registry) AllResources() map[string][]Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string][]Resource)
	for name, c := range r.clients {
		if c.IsConnected() {
			result[name] = c.ListResources()
		}
	}
	return result
}

func (r *Registry) AllPrompts() map[string][]Prompt {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string][]Prompt)
	for name, c := range r.clients {
		if c.IsConnected() {
			result[name] = c.ListPrompts()
		}
	}
	return result
}

func (r *Registry) CallTool(ctx context.Context, clientName, toolName string, args map[string]any) (any, error) {
	r.mu.RLock()
	c, ok := r.clients[clientName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("MCP client not found: %s", clientName)
	}
	if !c.IsConnected() {
		return nil, fmt.Errorf("MCP client not connected: %s", clientName)
	}
	return c.CallTool(ctx, toolName, args)
}

func (r *Registry) ReadResource(ctx context.Context, clientName, uri string) (any, error) {
	r.mu.RLock()
	c, ok := r.clients[clientName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("MCP client not found: %s", clientName)
	}
	if !c.IsConnected() {
		return nil, fmt.Errorf("MCP client not connected: %s", clientName)
	}
	return c.ReadResource(ctx, uri)
}

func (r *Registry) GetPrompt(ctx context.Context, clientName, name string, args map[string]string) (any, error) {
	r.mu.RLock()
	c, ok := r.clients[clientName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("MCP client not found: %s", clientName)
	}
	if !c.IsConnected() {
		return nil, fmt.Errorf("MCP client not connected: %s", clientName)
	}
	return c.GetPrompt(ctx, name, args)
}

func (r *Registry) Status() map[string]ServerStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]ServerStatus)
	for name, c := range r.clients {
		status := ServerStatus{Name: name, Connected: c.IsConnected()}
		if c.IsConnected() {
			status.Tools = len(c.ListTools())
			status.Resources = len(c.ListResources())
			status.Prompts = len(c.ListPrompts())
		}
		result[name] = status
	}
	return result
}

type ServerStatus struct {
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	Tools     int    `json:"tools"`
	Resources int    `json:"resources"`
	Prompts   int    `json:"prompts"`
}
