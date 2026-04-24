package reply

import (
	"context"
	"sync"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
)

type Dispatcher struct {
	mu         sync.RWMutex
	agents     map[string]*AgentHandler
	commands   map[string]CommandHandler
	hooks      []Hook
	toolReg    *tools.Registry
	llmClients map[string]llm.Client
}

type AgentHandler struct {
	Name        string
	Description string
	Model       string
	Provider    string
}

type CommandHandler struct {
	Name        string
	Description string
	Handler     func(ctx context.Context, args map[string]string) (string, error)
	Auth        string
}

type Hook interface {
	OnMessage(ctx context.Context, msg *Message) error
	OnResponse(ctx context.Context, resp *Response) error
}

type Message struct {
	ID        string
	Channel   string
	From      string
	Text      string
	Timestamp int64
	Metadata  map[string]any
}

type Response struct {
	MessageID string
	Text      string
	Tools     []ToolCall
	Metadata  map[string]any
}

type ToolCall struct {
	Name      string
	Arguments map[string]any
	ID        string
}

func NewDispatcher(toolReg *tools.Registry) *Dispatcher {
	return &Dispatcher{
		agents:     make(map[string]*AgentHandler),
		commands:   make(map[string]CommandHandler),
		hooks:      make([]Hook, 0),
		toolReg:    toolReg,
		llmClients: make(map[string]llm.Client),
	}
}

func (d *Dispatcher) RegisterAgent(agent *AgentHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agents[agent.Name] = agent
}

func (d *Dispatcher) RegisterCommand(cmd CommandHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.commands[cmd.Name] = cmd
}

func (d *Dispatcher) RegisterHook(hook Hook) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hooks = append(d.hooks, hook)
}

func (d *Dispatcher) RegisterLLM(name string, client llm.Client) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.llmClients[name] = client
}

func (d *Dispatcher) Dispatch(ctx context.Context, msg *Message) (*Response, error) {
	for _, hook := range d.hooks {
		if err := hook.OnMessage(ctx, msg); err != nil {
			return nil, err
		}
	}

	if cmd, ok := d.parseCommand(msg.Text); ok {
		if handler, exists := d.commands[cmd.Name]; exists {
			result, err := handler.Handler(ctx, cmd.Args)
			if err != nil {
				return nil, err
			}
			return &Response{
				MessageID: msg.ID,
				Text:      result,
			}, nil
		}
	}

	agentName := d.resolveAgent(msg)
	if agent, ok := d.agents[agentName]; ok {
		return d.callAgent(ctx, msg, agent)
	}

	return &Response{
		MessageID: msg.ID,
		Text:      "No agent available",
	}, nil
}

func (d *Dispatcher) parseCommand(text string) (cmd Command, ok bool) {
	if len(text) > 1 && text[0] == '/' {
		parts := splitArgs(text[1:])
		if len(parts) > 0 {
			cmd.Name = parts[0]
			cmd.Args = make(map[string]string)
			for i := 1; i < len(parts); i++ {
				cmd.Args[parts[i]] = ""
			}
			return cmd, true
		}
	}
	return
}

type Command struct {
	Name string
	Args map[string]string
}

func splitArgs(s string) []string {
	var args []string
	var current string
	inQuote := false

	for _, c := range s {
		if c == '"' {
			inQuote = !inQuote
		} else if c == ' ' && !inQuote {
			if current != "" {
				args = append(args, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		args = append(args, current)
	}
	return args
}

func (d *Dispatcher) resolveAgent(msg *Message) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for name := range d.agents {
		return name
	}
	return ""
}

func (d *Dispatcher) callAgent(ctx context.Context, msg *Message, agent *AgentHandler) (*Response, error) {
	d.mu.RLock()
	client, ok := d.llmClients[agent.Provider]
	d.mu.RUnlock()

	if !ok || client == nil {
		return &Response{MessageID: msg.ID, Text: "Provider not available"}, nil
	}

	messages := []llm.Message{
		{Role: "user", Content: msg.Text},
	}

	resp, err := client.Chat(ctx, messages, nil)
	if err != nil {
		return nil, err
	}

	return &Response{
		MessageID: msg.ID,
		Text:      resp.Content,
	}, nil
}
