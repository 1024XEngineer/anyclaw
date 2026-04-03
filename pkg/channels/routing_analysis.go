package channel

import (
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type RuleWarning struct {
	Index   int    `json:"index"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func AnalyzeRouting(cfg config.RoutingConfig) []RuleWarning {
	var warnings []RuleWarning
	for i, rule := range cfg.Rules {
		for j := 0; j < i; j++ {
			prev := cfg.Rules[j]
			if sameRule(prev, rule) {
				warnings = append(warnings, RuleWarning{Index: i, Kind: "duplicate", Message: "duplicate of earlier rule"})
				continue
			}
			if shadows(prev, rule) {
				warnings = append(warnings, RuleWarning{Index: i, Kind: "shadowed", Message: "earlier broader rule may shadow this rule"})
			}
		}
		if strings.TrimSpace(rule.Match) == "" {
			warnings = append(warnings, RuleWarning{Index: i, Kind: "broad", Message: "rule matches all messages for this channel"})
		}
	}
	return warnings
}

func sameRule(a, b config.ChannelRoutingRule) bool {
	return a.Channel == b.Channel &&
		a.Match == b.Match &&
		a.SessionMode == b.SessionMode &&
		a.Agent == b.Agent &&
		a.Org == b.Org &&
		a.Project == b.Project &&
		firstNonEmptyString(a.WorkspaceRef, a.Workspace) == firstNonEmptyString(b.WorkspaceRef, b.Workspace)
}

func shadows(a, b config.ChannelRoutingRule) bool {
	if a.Channel != b.Channel {
		return false
	}
	if strings.TrimSpace(a.Match) == "" && strings.TrimSpace(b.Match) != "" {
		return true
	}
	if strings.TrimSpace(a.Match) != "" && strings.Contains(strings.TrimSpace(b.Match), strings.TrimSpace(a.Match)) {
		return true
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
