package routing

import (
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type Decision struct {
	Provider string
	Model    string
	Reason   string
}

func DecideLLM(cfg config.LLMConfig, input string) Decision {
	decision := Decision{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		Reason:   "default",
	}
	route := cfg.Routing
	if !route.Enabled {
		return decision
	}
	lower := strings.ToLower(strings.TrimSpace(input))
	for _, keyword := range route.ReasoningKeywords {
		if keyword != "" && strings.Contains(lower, strings.ToLower(keyword)) {
			if route.ReasoningProvider != "" {
				decision.Provider = route.ReasoningProvider
			}
			if route.ReasoningModel != "" {
				decision.Model = route.ReasoningModel
			}
			decision.Reason = "reasoning"
			return decision
		}
	}
	if route.FastProvider != "" {
		decision.Provider = route.FastProvider
	}
	if route.FastModel != "" {
		decision.Model = route.FastModel
	}
	decision.Reason = "fast"
	return decision
}
