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

func NewPluginContext(name string, version string, workingDir string, gatewayAddr string, cfg map[string]any) *PluginContext {
	if cfg == nil {
		cfg = map[string]any{}
	}
	return &PluginContext{
		Name:        name,
		Version:     version,
		WorkingDir:  workingDir,
		Config:      cfg,
		GatewayAddr: gatewayAddr,
		tools:       make(map[string]Tool),
		channels:    make(map[string]Channel),
		handlers:    make(map[string]EventHandler),
		httpRoutes:  make(map[string]HTTPRoute),
		nodes:       make(map[string]Node),
	}
}

func NewPluginAPI(ctx *PluginContext) *PluginAPI {
	if ctx == nil {
		ctx = NewPluginContext("", "", "", "", nil)
	}
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

func (p *PluginAPI) ListTools() []Tool {
	p.ctx.mu.RLock()
	defer p.ctx.mu.RUnlock()

	items := make([]Tool, 0, len(p.ctx.tools))
	for _, tool := range p.ctx.tools {
		items = append(items, tool)
	}
	return items
}

func (p *PluginAPI) ListChannels() []Channel {
	p.ctx.mu.RLock()
	defer p.ctx.mu.RUnlock()

	items := make([]Channel, 0, len(p.ctx.channels))
	for _, ch := range p.ctx.channels {
		items = append(items, ch)
	}
	return items
}

func (p *PluginAPI) ListHTTPRoutes() []HTTPRoute {
	p.ctx.mu.RLock()
	defer p.ctx.mu.RUnlock()

	items := make([]HTTPRoute, 0, len(p.ctx.httpRoutes))
	for _, route := range p.ctx.httpRoutes {
		items = append(items, route)
	}
	return items
}

func (p *PluginAPI) ListNodes() []Node {
	p.ctx.mu.RLock()
	defer p.ctx.mu.RUnlock()

	items := make([]Node, 0, len(p.ctx.nodes))
	for _, node := range p.ctx.nodes {
		items = append(items, node)
	}
	return items
}

type PluginManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Kind        []string `json:"kind"`
	Entrypoint  string   `json:"entrypoint,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

func (m PluginManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("plugin manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("plugin manifest: version is required")
	}
	if len(m.Kind) == 0 {
		return fmt.Errorf("plugin manifest: at least one kind is required")
	}
	return nil
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
