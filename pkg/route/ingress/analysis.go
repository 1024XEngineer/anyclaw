package ingress

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

// RuleWarning describes a suspicious routing-rule condition.
type RuleWarning struct {
	Index   int    `json:"index"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// AnalyzeRouting returns warnings for duplicate, shadowed, or too-broad rules.
func AnalyzeRouting(cfg config.RoutingConfig) []RuleWarning {
	var warnings []RuleWarning
	mode := defaultString(cfg.Mode, "per-chat")
	for i, rule := range cfg.Rules {
		for j := 0; j < i; j++ {
			prev := cfg.Rules[j]
			if sameRule(prev, rule, mode) {
				warnings = append(warnings, RuleWarning{Index: i, Kind: "duplicate", Message: "duplicate of earlier rule"})
				continue
			}
			if shadows(prev, rule) {
				warnings = append(warnings, RuleWarning{Index: i, Kind: "shadowed", Message: "earlier broader rule may shadow this rule"})
			}
		}
		if rule.Match == "" {
			warnings = append(warnings, RuleWarning{Index: i, Kind: "broad", Message: "rule matches all messages for this channel"})
		}
	}
	return warnings
}

func sameRule(a, b config.ChannelRoutingRule, mode string) bool {
	aReplyBack := a.ReplyBack != nil && *a.ReplyBack
	bReplyBack := b.ReplyBack != nil && *b.ReplyBack

	return sameChannel(a.Channel, b.Channel) &&
		a.Match == b.Match &&
		defaultString(a.SessionMode, mode) == defaultString(b.SessionMode, mode) &&
		strings.TrimSpace(a.SessionID) == strings.TrimSpace(b.SessionID) &&
		strings.TrimSpace(a.QueueMode) == strings.TrimSpace(b.QueueMode) &&
		strings.TrimSpace(a.TitlePrefix) == strings.TrimSpace(b.TitlePrefix) &&
		aReplyBack == bReplyBack
}

func shadows(a, b config.ChannelRoutingRule) bool {
	if !sameChannel(a.Channel, b.Channel) {
		return false
	}
	if a.Match == "" && b.Match != "" {
		return true
	}
	if a.Match != "" && strings.Contains(b.Match, a.Match) {
		return true
	}
	return false
}

func sameChannel(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
