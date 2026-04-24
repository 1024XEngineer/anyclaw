package intake

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type MainEntryPolicy struct {
	ResolveMainAgentName func() string
}

func RequestedAgentName(agentName string, assistantName string) string {
	if trimmed := strings.TrimSpace(agentName); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(assistantName)
}

func (p MainEntryPolicy) NormalizeRequestedAgent(agentName string, assistantName string) (string, error) {
	return p.NormalizeAgent(RequestedAgentName(agentName, assistantName))
}

func (p MainEntryPolicy) NormalizeAgent(name string) (string, error) {
	mainAgentName, err := p.mainAgentName()
	if err != nil {
		return "", err
	}

	name = strings.TrimSpace(name)
	if name == "" || config.IsMainAgentAlias(name) || strings.EqualFold(name, mainAgentName) {
		return mainAgentName, nil
	}
	return "", fmt.Errorf("external entry only supports the main agent; specialist %q must be invoked through main agent handoff", name)
}

func (p MainEntryPolicy) NormalizeSelectionList(names ...string) ([]string, error) {
	mainAgentName, err := p.mainAgentName()
	if err != nil {
		return nil, err
	}

	hasSelection := false
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		hasSelection = true
		if config.IsMainAgentAlias(name) || strings.EqualFold(name, mainAgentName) {
			continue
		}
		return nil, fmt.Errorf("external entry only supports the main agent; specialist %q must be invoked through main agent handoff", name)
	}
	if !hasSelection {
		return nil, nil
	}
	return []string{mainAgentName}, nil
}

func (p MainEntryPolicy) mainAgentName() (string, error) {
	if p.ResolveMainAgentName == nil {
		return "", fmt.Errorf("main agent not configured")
	}
	mainAgentName := strings.TrimSpace(p.ResolveMainAgentName())
	if mainAgentName == "" {
		return "", fmt.Errorf("main agent not configured")
	}
	return mainAgentName, nil
}
