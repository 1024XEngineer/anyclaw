package clihub

import (
	"fmt"
	"strings"
)

type Intent struct {
	Action     string
	Target     string
	Subject    string
	Parameters map[string]string
	Confidence float64
	Capability *Capability
	Args       []string
}

type IntentEngine struct {
	registry *CapabilityRegistry
}

func NewIntentEngine(root string) (*IntentEngine, error) {
	reg, err := LoadCapabilityRegistry(root)
	if err != nil {
		return nil, err
	}
	return &IntentEngine{registry: reg}, nil
}

func (e *IntentEngine) Parse(query string) *Intent {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	cap := e.registry.BestMatch(query)
	if cap == nil {
		return nil
	}

	intent := &Intent{
		Action:     cap.Action,
		Target:     cap.Harness,
		Subject:    cap.Command,
		Parameters: make(map[string]string),
		Confidence: 1.0,
		Capability: cap,
	}

	return intent
}

func (e *IntentEngine) ParseWithArgs(query string, args []string) *Intent {
	intent := e.Parse(query)
	if intent == nil {
		return nil
	}
	intent.Args = args
	return intent
}

func (e *IntentEngine) ResolveCommand(intent *Intent) ([]string, string, error) {
	if intent == nil || intent.Capability == nil {
		return nil, "", fmt.Errorf("no intent to resolve")
	}
	return ResolveCapabilityPath("", *intent.Capability)
}

func (e *IntentEngine) GetCapability(name string) *Capability {
	for _, cap := range e.registry.Capabilities {
		fullName := fmt.Sprintf("%s_%s", cap.Harness, cap.Command)
		if strings.EqualFold(fullName, name) {
			return &cap
		}
	}
	return nil
}

func (e *IntentEngine) AllCapabilities() []Capability {
	return e.registry.All()
}

func (e *IntentEngine) Count() int {
	return e.registry.Count()
}

func (e *IntentEngine) Search(query string) []Capability {
	return e.registry.FindByIntent(query)
}

type IntentRouter struct {
	engine   *IntentEngine
	handlers map[string]IntentHandler
	Registry *CapabilityRegistry
}

type IntentHandler func(intent *Intent) (string, error)

func NewIntentRouter(root string) (*IntentRouter, error) {
	engine, err := NewIntentEngine(root)
	if err != nil {
		return nil, err
	}
	return &IntentRouter{
		engine:   engine,
		handlers: make(map[string]IntentHandler),
		Registry: engine.registry,
	}, nil
}

func (r *IntentRouter) RegisterHandler(harness string, handler IntentHandler) {
	r.handlers[strings.ToLower(harness)] = handler
}

func (r *IntentRouter) Route(query string, args []string) (string, error) {
	intent := r.engine.ParseWithArgs(query, args)
	if intent == nil {
		return "", fmt.Errorf("could not understand intent: %s", query)
	}

	harnessLower := strings.ToLower(intent.Target)
	if handler, ok := r.handlers[harnessLower]; ok {
		return handler(intent)
	}

	return "", fmt.Errorf("no handler for harness: %s", intent.Target)
}

func (r *IntentRouter) ExecuteCapability(capName string, args []string, execFunc func([]string, string) (string, error)) (string, error) {
	cap := r.engine.GetCapability(capName)
	if cap == nil {
		return "", fmt.Errorf("capability not found: %s", capName)
	}

	baseCmd, cwd, err := ResolveCapabilityPath("", *cap)
	if err != nil {
		return "", err
	}

	fullArgs := append(baseCmd, cap.Command)
	fullArgs = append(fullArgs, args...)

	return execFunc(fullArgs, cwd)
}

func (r *IntentRouter) ListCapabilities() []Capability {
	return r.engine.AllCapabilities()
}

func HumanIntentSummary(engine *IntentEngine) string {
	if engine == nil {
		return "Intent engine not initialized"
	}

	lines := []string{
		fmt.Sprintf("Intent Engine: %d capabilities loaded", engine.Count()),
		"",
		"Example intents you can express:",
	}

	examples := []string{
		"create a new video project",
		"list all models in ollama",
		"export the current project",
		"add a clip to timeline",
		"search for files",
		"open libreoffice spreadsheet",
	}

	for _, ex := range examples {
		intent := engine.Parse(ex)
		if intent != nil {
			lines = append(lines, fmt.Sprintf("  \"%s\" -> %s/%s", ex, intent.Target, intent.Subject))
		}
	}

	return strings.Join(lines, "\n")
}

func ExtractEntities(query string) map[string]string {
	entities := make(map[string]string)

	query = strings.ToLower(query)

	entityPatterns := map[string][]string{
		"file":   {"file", "project", "document", "spreadsheet", "presentation"},
		"action": {"create", "list", "add", "remove", "export", "import", "open", "save"},
		"target": {"video", "audio", "image", "text", "document"},
	}

	for entity, patterns := range entityPatterns {
		for _, pattern := range patterns {
			if strings.Contains(query, pattern) {
				entities[entity] = pattern
			}
		}
	}

	return entities
}
