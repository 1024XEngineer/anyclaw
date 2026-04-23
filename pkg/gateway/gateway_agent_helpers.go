package gateway

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Server) resolveAgentName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if resolved := s.mainRuntime.Config.ResolveMainAgentName(); resolved != "" {
			return resolved, nil
		}
		return "", fmt.Errorf("agent not configured")
	}
	if config.IsMainAgentAlias(name) {
		if resolved := s.mainRuntime.Config.ResolveMainAgentName(); resolved != "" {
			return resolved, nil
		}
		return "", fmt.Errorf("agent not configured")
	}
	profile, ok := s.mainRuntime.Config.FindAgentProfile(name)
	if !ok {
		if strings.EqualFold(name, strings.TrimSpace(s.mainRuntime.Config.ResolveMainAgentName())) {
			return s.mainRuntime.Config.ResolveMainAgentName(), nil
		}
		return "", fmt.Errorf("agent not found: %s", name)
	}
	if !profile.IsEnabled() {
		return "", fmt.Errorf("agent is disabled: %s", name)
	}
	return profile.Name, nil
}
