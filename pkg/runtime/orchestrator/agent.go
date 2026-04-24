package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/isolation"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

type AgentDefinition struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Role              string   `json:"role,omitempty"`
	ParentRef         string   `json:"parent_ref,omitempty"`
	Persona           string   `json:"persona,omitempty"`
	Domain            string   `json:"domain,omitempty"`
	Expertise         []string `json:"expertise,omitempty"`
	SystemPrompt      string   `json:"system_prompt,omitempty"`
	ConversationTone  string   `json:"conversation_tone,omitempty"`
	ConversationStyle string   `json:"conversation_style,omitempty"`
	PrivateSkills     []string `json:"private_skills,omitempty"`
	PermissionLevel   string   `json:"permission_level"`
	WorkingDir        string   `json:"working_dir,omitempty"`

	LLMProvider    string   `json:"llm_provider,omitempty"`
	LLMModel       string   `json:"llm_model,omitempty"`
	LLMAPIKey      string   `json:"llm_api_key,omitempty"`
	LLMBaseURL     string   `json:"llm_base_url,omitempty"`
	LLMMaxTokens   *int     `json:"llm_max_tokens,omitempty"`
	LLMTemperature *float64 `json:"llm_temperature,omitempty"`
	LLMProxy       string   `json:"llm_proxy,omitempty"`

	ContextIsolationMode     string `json:"context_isolation_mode,omitempty"`
	ContextVisibility        string `json:"context_visibility,omitempty"`
	ContextNamespace         string `json:"context_namespace,omitempty"`
	ContextMaxSize           int    `json:"context_max_size,omitempty"`
	ContextTTLSeconds        int    `json:"context_ttl_seconds,omitempty"`
	ContextAllowSharing      bool   `json:"context_allow_sharing,omitempty"`
	ContextInheritFromParent bool   `json:"context_inherit_from_parent,omitempty"`
	ContextParentScopeID     string `json:"context_parent_scope_id,omitempty"`
}

type SubAgent struct {
	definition      AgentDefinition
	agent           *agent.Agent
	llmClient       agent.LLMCaller
	skills          *skills.SkillsManager
	tools           *tools.Registry
	memory          memory.MemoryBackend
	mu              sync.Mutex
	lastResult      string
	lastError       error
	execCount       int
	contextEngine   *isolation.IsolatedEngine
	contextBoundary *isolation.ContextBoundary
	contextScopeID  string
	lifecycleID     string
	messageBus      *MessageBus
}

type LLMConfig struct {
	Provider    string
	Model       string
	APIKey      string
	BaseURL     string
	MaxTokens   int
	Temperature float64
	Proxy       string
}

func NewSubAgent(def AgentDefinition, llmClient agent.LLMCaller, allSkills *skills.SkillsManager, baseTools *tools.Registry, mem memory.MemoryBackend) (*SubAgent, error) {
	return NewSubAgentWithContext(def, llmClient, allSkills, baseTools, mem, nil, "")
}

func NewSubAgentWithContext(def AgentDefinition, llmClient agent.LLMCaller, allSkills *skills.SkillsManager, baseTools *tools.Registry, mem memory.MemoryBackend, isoManager *isolation.ContextIsolationManager, parentScopeID string) (*SubAgent, error) {
	if strings.TrimSpace(def.Name) == "" {
		return nil, fmt.Errorf("agent name is required")
	}

	permLevel := strings.TrimSpace(def.PermissionLevel)
	if permLevel == "" {
		permLevel = "limited"
	}

	effectiveLLM := llmClient
	if def.LLMProvider != "" {
		maxTokens := 0
		if def.LLMMaxTokens != nil {
			maxTokens = *def.LLMMaxTokens
		}
		temperature := 0.0
		if def.LLMTemperature != nil {
			temperature = *def.LLMTemperature
		}
		cfg := llm.Config{
			Provider:    def.LLMProvider,
			Model:       def.LLMModel,
			APIKey:      def.LLMAPIKey,
			BaseURL:     def.LLMBaseURL,
			MaxTokens:   maxTokens,
			Temperature: temperature,
			Proxy:       def.LLMProxy,
		}
		if cfg.Model == "" {
			cfg.Model = "gpt-4o-mini"
		}
		customClient, err := llm.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM client for agent %s: %w", def.Name, err)
		}
		effectiveLLM = customClient
	}

	// Build private skills manager
	var privateSkills *skills.SkillsManager
	if len(def.PrivateSkills) > 0 && allSkills != nil {
		privateSkills = allSkills.FilterEnabled(def.PrivateSkills)
	} else if allSkills != nil {
		privateSkills = allSkills
	} else {
		privateSkills = skills.NewSkillsManager("")
	}

	// Build private tool registry filtered by permission
	privateTools := tools.NewRegistry()
	if baseTools != nil {
		for _, tool := range baseTools.ListToolsForRole(true) {
			if isToolAllowedForPermission(tool.Name, permLevel) {
				privateTools.Register(tool)
			}
		}
	}

	// Register skills as tools
	if privateSkills != nil {
		privateSkills.RegisterTools(privateTools, skills.ExecutionOptions{AllowExec: true, ExecTimeoutSeconds: 30})
	}

	// Each agent gets its own memory instance for isolation
	var agentMem memory.MemoryBackend
	if mem != nil && strings.TrimSpace(def.WorkingDir) != "" {
		subCfg := memory.DefaultConfig(def.WorkingDir)
		subMem, err := memory.NewMemoryBackend(subCfg)
		if err == nil {
			if initErr := subMem.Init(); initErr == nil {
				agentMem = subMem
			} else {
				agentMem = mem // fallback
			}
		} else {
			agentMem = mem // fallback
		}
	} else {
		agentMem = mem
	}

	// Build the full personality prompt from agent definition
	personality := buildAgentPersonality(def)

	var isolatedEngine *isolation.IsolatedEngine
	var boundary *isolation.ContextBoundary
	scopeID := ""

	if isoManager != nil {
		mode := isolation.IsolationMode(def.ContextIsolationMode)
		if mode == "" {
			mode = isolation.IsolationModeStrict
		}

		visibility := isolation.ContextVisibility(def.ContextVisibility)
		if visibility == "" {
			visibility = isolation.ContextVisibilityPrivate
		}

		namespace := def.ContextNamespace
		if namespace == "" {
			namespace = def.Name
		}

		maxSize := def.ContextMaxSize
		if maxSize <= 0 {
			maxSize = 1000
		}
		_ = maxSize

		scope := &isolation.ContextScope{
			AgentID:   def.Name,
			SessionID: "",
			TaskID:    "",
			Namespace: namespace,
			Labels: map[string]string{
				"domain":     def.Domain,
				"permission": def.PermissionLevel,
			},
		}

		var err error
		if def.ContextInheritFromParent && parentScopeID != "" {
			boundary, err = isoManager.CreateChildBoundary(parentScopeID, scope, mode, visibility)
		} else if def.ContextParentScopeID != "" {
			boundary, err = isoManager.CreateChildBoundary(def.ContextParentScopeID, scope, mode, visibility)
		} else {
			boundary, err = isoManager.CreateBoundary(scope, mode, visibility)
		}

		if err == nil {
			scopeID = scope.ID()
			if engine, ok := isoManager.GetEngine(scopeID); ok {
				isolatedEngine = engine
			}
		}
	}

	// Create the underlying agent
	ag := agent.New(agent.Config{
		Name:             def.Name,
		Description:      def.Description,
		Personality:      personality,
		IsSubAgent:       true,
		LLM:              effectiveLLM,
		Memory:           agentMem,
		Skills:           privateSkills,
		Tools:            privateTools,
		WorkDir:          def.WorkingDir,
		WorkingDir:       def.WorkingDir,
		MaxContextTokens: maxTokensForAgent(def),
		ContextEngine:    isolatedEngine,
	})

	subAgent := &SubAgent{
		definition:      def,
		agent:           ag,
		llmClient:       effectiveLLM,
		skills:          privateSkills,
		tools:           privateTools,
		memory:          agentMem,
		contextEngine:   isolatedEngine,
		contextBoundary: boundary,
		contextScopeID:  scopeID,
	}

	return subAgent, nil
}

func maxTokensForAgent(def AgentDefinition) int {
	if def.LLMMaxTokens != nil && *def.LLMMaxTokens > 0 {
		return *def.LLMMaxTokens
	}
	return 4096
}

func buildAgentPersonality(def AgentDefinition) string {
	var parts []string

	// System prompt takes priority - this is the agent's full identity
	if strings.TrimSpace(def.SystemPrompt) != "" {
		parts = append(parts, def.SystemPrompt)
	}

	// Persona
	if strings.TrimSpace(def.Persona) != "" {
		parts = append(parts, "角色: "+def.Persona)
	}

	// Domain
	if strings.TrimSpace(def.Domain) != "" {
		parts = append(parts, "领域: "+def.Domain)
	}

	// Expertise
	if len(def.Expertise) > 0 {
		parts = append(parts, "擅长: "+strings.Join(def.Expertise, "、"))
	}

	// Conversation style
	if strings.TrimSpace(def.ConversationTone) != "" {
		parts = append(parts, "语气: "+def.ConversationTone)
	}
	if strings.TrimSpace(def.ConversationStyle) != "" {
		parts = append(parts, "风格: "+def.ConversationStyle)
	}

	return strings.Join(parts, "\n\n")
}

func isToolAllowedForPermission(toolName string, permLevel string) bool {
	switch permLevel {
	case "full":
		return true
	case "read-only":
		switch toolName {
		case "read_file", "list_directory", "search_files",
			"web_search", "fetch_url",
			"browser_navigate", "browser_screenshot", "browser_snapshot",
			"browser_click", "browser_wait", "browser_scroll",
			"browser_tab_list", "browser_tab_new", "browser_tab_switch", "browser_tab_close",
			"browser_close", "browser_eval", "browser_select", "browser_press", "browser_type",
			"desktop_screenshot", "desktop_screenshot_window", "desktop_list_windows", "desktop_wait_window", "desktop_inspect_ui", "desktop_resolve_target", "desktop_match_image", "desktop_wait_image", "desktop_ocr", "desktop_verify_text", "desktop_find_text", "desktop_wait_text", "desktop_clipboard_get":
			return true
		default:
			return !strings.HasPrefix(toolName, "write_") &&
				!strings.HasPrefix(toolName, "run_command") &&
				!strings.HasPrefix(toolName, "desktop_") &&
				toolName != "browser_upload" &&
				toolName != "browser_download" &&
				toolName != "browser_pdf"
		}
	default: // limited
		return true
	}
}

func (sa *SubAgent) Run(ctx context.Context, input string) (string, error) {
	sa.mu.Lock()
	sa.execCount++
	sa.mu.Unlock()

	result, err := sa.agent.Run(ctx, input)

	sa.mu.Lock()
	sa.lastResult = result
	sa.lastError = err
	sa.mu.Unlock()

	return result, err
}

func (sa *SubAgent) Name() string {
	return sa.definition.Name
}

func (sa *SubAgent) Description() string {
	return sa.definition.Description
}

func (sa *SubAgent) Role() string {
	return sa.definition.Role
}

func (sa *SubAgent) ParentRef() string {
	return sa.definition.ParentRef
}

func (sa *SubAgent) Domain() string {
	return sa.definition.Domain
}

func (sa *SubAgent) Persona() string {
	return sa.definition.Persona
}

func (sa *SubAgent) Expertise() []string {
	return sa.definition.Expertise
}

func (sa *SubAgent) Skills() []string {
	if sa.skills == nil {
		return nil
	}
	list := sa.skills.List()
	names := make([]string, len(list))
	for i, s := range list {
		names[i] = s.Name
	}
	return names
}

func (sa *SubAgent) PermissionLevel() string {
	return sa.definition.PermissionLevel
}

func (sa *SubAgent) ExecCount() int {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	return sa.execCount
}

func (sa *SubAgent) HasSkill(name string) bool {
	if sa.skills == nil {
		return false
	}
	_, ok := sa.skills.Get(name)
	return ok
}

func (sa *SubAgent) Definition() AgentDefinition {
	return sa.definition
}

func (sa *SubAgent) ContextEngine() *isolation.IsolatedEngine {
	return sa.contextEngine
}

func (sa *SubAgent) ContextBoundary() *isolation.ContextBoundary {
	return sa.contextBoundary
}

func (sa *SubAgent) ContextScopeID() string {
	return sa.contextScopeID
}

func (sa *SubAgent) HasIsolatedContext() bool {
	return sa.contextEngine != nil
}

func (sa *SubAgent) SetLifecycleID(id string) {
	sa.lifecycleID = id
}

func (sa *SubAgent) LifecycleID() string {
	return sa.lifecycleID
}

func (sa *SubAgent) SetMessageBus(mb *MessageBus) {
	sa.messageBus = mb
}

func (sa *SubAgent) SendMessage(to string, msgType string, payload map[string]any) {
	if sa.messageBus == nil {
		return
	}
	sa.messageBus.Send(&AgentMessage{
		From:    sa.Name(),
		To:      to,
		Type:    msgType,
		Payload: payload,
	})
}

func (sa *SubAgent) BroadcastMessage(msgType string, payload map[string]any) {
	if sa.messageBus == nil {
		return
	}
	sa.messageBus.Send(&AgentMessage{
		From:      sa.Name(),
		Broadcast: true,
		Type:      msgType,
		Payload:   payload,
	})
}

type AgentInfo struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Role              string   `json:"role,omitempty"`
	ParentRef         string   `json:"parent_ref,omitempty"`
	Persona           string   `json:"persona,omitempty"`
	Domain            string   `json:"domain,omitempty"`
	Expertise         []string `json:"expertise,omitempty"`
	Skills            []string `json:"skills,omitempty"`
	PermissionLevel   string   `json:"permission_level,omitempty"`
	LLMProvider       string   `json:"llm_provider,omitempty"`
	LLMModel          string   `json:"llm_model,omitempty"`
	ExecCount         int      `json:"exec_count"`
	ContextIsolation  string   `json:"context_isolation,omitempty"`
	ContextVisibility string   `json:"context_visibility,omitempty"`
	ContextScopeID    string   `json:"context_scope_id,omitempty"`
	ContextDocCount   int      `json:"context_doc_count,omitempty"`
}

type AgentPool struct {
	mu     sync.RWMutex
	agents map[string]*SubAgent
}

func NewAgentPool() *AgentPool {
	return &AgentPool{
		agents: make(map[string]*SubAgent),
	}
}

func (p *AgentPool) Register(name string, sa *SubAgent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.agents[name] = sa
}

func (p *AgentPool) Unregister(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.agents, name)
}

func (p *AgentPool) Get(name string) (*SubAgent, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sa, ok := p.agents[name]
	return sa, ok
}

func (p *AgentPool) FindAgentForSkills(requiredSkills []string) *SubAgent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	bestMatch := 0
	var bestAgent *SubAgent

	for _, sa := range p.agents {
		matchCount := 0
		for _, req := range requiredSkills {
			if sa.HasSkill(req) {
				matchCount++
			}
		}
		if matchCount > bestMatch {
			bestMatch = matchCount
			bestAgent = sa
		}
	}

	return bestAgent
}

func (p *AgentPool) FindAgentForDomain(domain string) *SubAgent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	domainLower := strings.TrimSpace(strings.ToLower(domain))
	if domainLower == "" {
		return nil
	}
	for _, sa := range p.agents {
		if strings.Contains(strings.ToLower(sa.definition.Domain), domainLower) {
			return sa
		}
		for _, exp := range sa.definition.Expertise {
			if strings.Contains(strings.ToLower(exp), domainLower) {
				return sa
			}
		}
	}
	return nil
}

func (p *AgentPool) List() []*SubAgent {
	p.mu.RLock()
	defer p.mu.RUnlock()
	list := make([]*SubAgent, 0, len(p.agents))
	for _, sa := range p.agents {
		list = append(list, sa)
	}
	return list
}

func (p *AgentPool) ListInfos() []AgentInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	list := make([]AgentInfo, 0, len(p.agents))
	for _, sa := range p.agents {
		info := AgentInfo{
			Name:              sa.Name(),
			Description:       sa.Description(),
			Role:              sa.Role(),
			ParentRef:         sa.ParentRef(),
			Persona:           sa.Persona(),
			Domain:            sa.Domain(),
			Expertise:         sa.Expertise(),
			Skills:            sa.Skills(),
			PermissionLevel:   sa.PermissionLevel(),
			LLMProvider:       sa.definition.LLMProvider,
			LLMModel:          sa.definition.LLMModel,
			ExecCount:         sa.ExecCount(),
			ContextIsolation:  string(sa.definition.ContextIsolationMode),
			ContextVisibility: string(sa.definition.ContextVisibility),
			ContextScopeID:    sa.contextScopeID,
		}
		if sa.contextEngine != nil {
			info.ContextDocCount = sa.contextEngine.DocumentCount()
		}
		list = append(list, info)
	}
	return list
}

func (p *AgentPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.agents)
}
