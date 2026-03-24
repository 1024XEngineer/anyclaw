package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type AgentDefinition struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Persona           string   `json:"persona,omitempty"`
	Domain            string   `json:"domain,omitempty"`
	Expertise         []string `json:"expertise,omitempty"`
	SystemPrompt      string   `json:"system_prompt,omitempty"`
	ConversationTone  string   `json:"conversation_tone,omitempty"`
	ConversationStyle string   `json:"conversation_style,omitempty"`
	PrivateSkills     []string `json:"private_skills,omitempty"`
	PermissionLevel   string   `json:"permission_level"`
	WorkingDir        string   `json:"working_dir,omitempty"`
}

type SubAgent struct {
	definition AgentDefinition
	agent      *agent.Agent
	skills     *skills.SkillsManager
	tools      *tools.Registry
	memory     *memory.FileMemory
	mu         sync.Mutex
	lastResult string
	lastError  error
	execCount  int
}

func NewSubAgent(def AgentDefinition, llmClient agent.LLMCaller, allSkills *skills.SkillsManager, baseTools *tools.Registry, mem *memory.FileMemory) (*SubAgent, error) {
	if strings.TrimSpace(def.Name) == "" {
		return nil, fmt.Errorf("agent name is required")
	}

	permLevel := strings.TrimSpace(def.PermissionLevel)
	if permLevel == "" {
		permLevel = "limited"
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
		for _, t := range baseTools.List() {
			if tool, ok := baseTools.Get(t.Name); ok {
				if isToolAllowedForPermission(t.Name, permLevel) {
					privateTools.Register(tool)
				}
			}
		}
	}

	// Register skills as tools
	if privateSkills != nil {
		privateSkills.RegisterTools(privateTools, skills.ExecutionOptions{AllowExec: true, ExecTimeoutSeconds: 30})
	}

	// Each agent gets its own memory instance for isolation
	var agentMem *memory.FileMemory
	if mem != nil && strings.TrimSpace(def.WorkingDir) != "" {
		agentMem = memory.NewFileMemory(def.WorkingDir)
		if err := agentMem.Init(); err != nil {
			agentMem = mem // fallback
		}
	} else {
		agentMem = mem
	}

	// Build the full personality prompt from agent definition
	personality := buildAgentPersonality(def)

	// Create the underlying agent
	ag := agent.New(agent.Config{
		Name:        def.Name,
		Description: def.Description,
		Personality: personality,
		LLM:         llmClient,
		Memory:      agentMem,
		Skills:      privateSkills,
		Tools:       privateTools,
		WorkDir:     def.WorkingDir,
	})

	return &SubAgent{
		definition: def,
		agent:      ag,
		skills:     privateSkills,
		tools:      privateTools,
		memory:     agentMem,
	}, nil
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
			"browser_close", "browser_eval", "browser_select", "browser_press", "browser_type":
			return true
		default:
			return !strings.HasPrefix(toolName, "write_") &&
				!strings.HasPrefix(toolName, "run_command") &&
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

type AgentInfo struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Persona         string   `json:"persona,omitempty"`
	Domain          string   `json:"domain,omitempty"`
	Expertise       []string `json:"expertise,omitempty"`
	Skills          []string `json:"skills,omitempty"`
	PermissionLevel string   `json:"permission_level,omitempty"`
	ExecCount       int      `json:"exec_count"`
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
		list = append(list, AgentInfo{
			Name:            sa.Name(),
			Description:     sa.Description(),
			Persona:         sa.Persona(),
			Domain:          sa.Domain(),
			Expertise:       sa.Expertise(),
			Skills:          sa.Skills(),
			PermissionLevel: sa.PermissionLevel(),
			ExecCount:       sa.ExecCount(),
		})
	}
	return list
}

func (p *AgentPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.agents)
}
