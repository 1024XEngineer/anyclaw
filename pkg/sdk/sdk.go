package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type PluginContext struct {
	Name        string
	Version     string
	WorkingDir  string
	Config      map[string]any
	GatewayAddr string

	mu         sync.RWMutex
	tools      map[string]Tool
	channels   map[string]Channel
	handlers   map[string]EventHandler
	httpRoutes map[string]HTTPRoute
	nodes      map[string]Node
}

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     ToolHandler
}

type ToolHandler func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)

type Channel interface {
	Name() string
	Start() error
	Stop() error
	Send(msg Message) error
	OnMessage(handler func(msg Message))
}

type Message struct {
	ID        string
	Channel   string
	From      string
	To        string
	Content   string
	Timestamp int64
	Metadata  map[string]any
}

type EventHandler func(ctx context.Context, event Event) error

type Event struct {
	Type    string
	Payload map[string]any
	Source  string
}

type HTTPRoute struct {
	Path    string
	Method  string
	Handler func(w http.ResponseWriter, r *http.Request)
}

type Node interface {
	Name() string
	Platform() string
	Connect() error
	Disconnect() error
	Invoke(action string, input json.RawMessage) (json.RawMessage, error)
	Capabilities() []string
}

type PluginAPI struct {
	ctx *PluginContext
}

func NewPluginAPI(ctx *PluginContext) *PluginAPI {
	return &PluginAPI{ctx: ctx}
}

func (p *PluginAPI) RegisterTool(tool Tool) error {
	p.ctx.mu.Lock()
	defer p.ctx.mu.Unlock()

	if p.ctx.tools == nil {
		p.ctx.tools = make(map[string]Tool)
	}
	if _, exists := p.ctx.tools[tool.Name]; exists {
		return fmt.Errorf("tool %s already registered", tool.Name)
	}
	p.ctx.tools[tool.Name] = tool
	return nil
}

func (p *PluginAPI) RegisterChannel(ch Channel) error {
	p.ctx.mu.Lock()
	defer p.ctx.mu.Unlock()

	if p.ctx.channels == nil {
		p.ctx.channels = make(map[string]Channel)
	}
	name := ch.Name()
	if _, exists := p.ctx.channels[name]; exists {
		return fmt.Errorf("channel %s already registered", name)
	}
	p.ctx.channels[name] = ch
	return nil
}

func (p *PluginAPI) RegisterEventHandler(eventType string, handler EventHandler) error {
	p.ctx.mu.Lock()
	defer p.ctx.mu.Unlock()

	if p.ctx.handlers == nil {
		p.ctx.handlers = make(map[string]EventHandler)
	}
	p.ctx.handlers[eventType] = handler
	return nil
}

func (p *PluginAPI) RegisterHTTPRoute(route HTTPRoute) error {
	p.ctx.mu.Lock()
	defer p.ctx.mu.Unlock()

	if p.ctx.httpRoutes == nil {
		p.ctx.httpRoutes = make(map[string]HTTPRoute)
	}
	key := route.Method + ":" + route.Path
	p.ctx.httpRoutes[key] = route
	return nil
}

func (p *PluginAPI) RegisterNode(node Node) error {
	p.ctx.mu.Lock()
	defer p.ctx.mu.Unlock()

	if p.ctx.nodes == nil {
		p.ctx.nodes = make(map[string]Node)
	}
	name := node.Name()
	if _, exists := p.ctx.nodes[name]; exists {
		return fmt.Errorf("node %s already registered", name)
	}
	p.ctx.nodes[name] = node
	return nil
}

func (p *PluginAPI) GetConfig(key string) (any, bool) {
	p.ctx.mu.RLock()
	defer p.ctx.mu.RUnlock()

	val, ok := p.ctx.Config[key]
	return val, ok
}

func (p *PluginAPI) SetConfig(key string, value any) {
	p.ctx.mu.Lock()
	defer p.ctx.mu.Unlock()

	p.ctx.Config[key] = value
}

func (p *PluginAPI) GetWorkingDir() string {
	return p.ctx.WorkingDir
}

func (p *PluginAPI) GetGatewayAddr() string {
	return p.ctx.GatewayAddr
}

type PluginManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Kind        []string `json:"kind"`
	Entrypoint  string   `json:"entrypoint,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

type PluginInitFunc func(api *PluginAPI) error
type PluginStartFunc func() error
type PluginStopFunc func() error

type Plugin struct {
	Manifest PluginManifest
	Init     PluginInitFunc
	Start    PluginStartFunc
	Stop     PluginStopFunc
}

func Register(manifest PluginManifest, initFn PluginInitFunc, startFn PluginStartFunc, stopFn PluginStopFunc) *Plugin {
	return &Plugin{
		Manifest: manifest,
		Init:     initFn,
		Start:    startFn,
		Stop:     stopFn,
	}
}
