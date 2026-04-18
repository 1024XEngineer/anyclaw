package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

func (o *Orchestrator) RunTemporaryPlan(ctx context.Context, brief string, requestedName string) (*OrchestratorResult, error) {
	if o == nil {
		return nil, fmt.Errorf("orchestrator is nil")
	}
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return nil, fmt.Errorf("handoff brief is required")
	}

	name, cleanup, err := o.registerTemporaryAgent(requestedName)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return o.runTaskResult(ctx, brief, []string{name}, true)
}

func (o *Orchestrator) registerTemporaryAgent(requestedName string) (string, func(), error) {
	if o.agentPool == nil {
		o.agentPool = NewAgentPool()
	}

	def := o.buildTemporaryAgentDefinition(requestedName)
	subAgent, err := NewSubAgent(def, o.llm, o.allSkills, o.baseTools, o.memory)
	if err != nil {
		return "", nil, fmt.Errorf("create temporary subagent %q: %w", def.Name, err)
	}

	if o.lifecycle != nil {
		if managed, lifecycleErr := o.lifecycle.Spawn(def.Name, "", map[string]string{
			"type":           "temporary_subagent",
			"requested_name": strings.TrimSpace(requestedName),
		}); lifecycleErr == nil {
			subAgent.SetLifecycleID(managed.ID)
		}
	}
	subAgent.SetMessageBus(o.messageBus)
	o.agentPool.Register(def.Name, subAgent)

	cleanup := func() {
		o.agentPool.Unregister(def.Name)
		if subAgent.memory != nil && subAgent.memory != o.memory {
			_ = subAgent.memory.Close()
		}
		if o.lifecycle != nil {
			_ = o.lifecycle.Cleanup()
		}
	}
	return def.Name, cleanup, nil
}

func (o *Orchestrator) buildTemporaryAgentDefinition(requestedName string) AgentDefinition {
	return AgentDefinition{
		Name:            o.uniqueTemporaryAgentName(requestedName),
		Description:     "Temporary delegated specialist created on demand for one routed task.",
		Role:            "temporary_subagent",
		Persona:         "Focused temporary specialist. Stay within the delegated scope and return concrete output to the main agent.",
		PermissionLevel: "limited",
		WorkingDir:      strings.TrimSpace(o.config.DefaultWorkingDir),
	}
}

func (o *Orchestrator) uniqueTemporaryAgentName(requestedName string) string {
	base := normalizeTemporaryAgentName(requestedName)
	if base == "" {
		base = "temporary-subagent"
	}
	if o.agentPool == nil {
		return base
	}
	if _, exists := o.agentPool.Get(base); !exists {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, exists := o.agentPool.Get(candidate); !exists {
			return candidate
		}
	}
}

func normalizeTemporaryAgentName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	return result
}
