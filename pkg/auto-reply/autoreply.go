package autoreply

import (
	"context"
	"fmt"
)

type Handler struct {
	rules []Rule
}

type Rule struct {
	Match   string
	Reply   string
	Pattern string
}

func New() *Handler {
	return &Handler{
		rules: make([]Rule, 0),
	}
}

func (h *Handler) Handle(ctx context.Context, message string) (string, error) {
	for _, rule := range h.rules {
		if rule.Pattern != "" {
			if matched := h.matchPattern(message, rule.Pattern); matched {
				return rule.Reply, nil
			}
		}
		if rule.Match != "" && contains(message, rule.Match) {
			return rule.Reply, nil
		}
	}
	return "", nil
}

func (h *Handler) matchPattern(message, pattern string) bool {
	return contains(message, pattern)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (h *Handler) AddRule(rule Rule) {
	h.rules = append(h.rules, rule)
}

func (h *Handler) RemoveRule(match string) {
	var newRules []Rule
	for _, r := range h.rules {
		if r.Match != match {
			newRules = append(newRules, r)
		}
	}
	h.rules = newRules
}

type AutoReplyConfig struct {
	Enabled  bool
	Rules    []Rule
	Response string
}

func (h *Handler) String() string {
	return fmt.Sprintf("AutoReply: %d rules", len(h.rules))
}
