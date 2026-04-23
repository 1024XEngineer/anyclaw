package ingress

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

// Router evaluates channel routing rules and returns the M2 route decision.
type Router struct {
	config config.RoutingConfig
}

// NewRouter creates the M2 rules router.
func NewRouter(cfg config.RoutingConfig) *Router {
	return &Router{config: cfg}
}

// Decide resolves one route request into a lightweight route decision.
func (r *Router) Decide(req RouteRequest) RouteDecision {
	if r == nil {
		return RouteDecision{}
	}

	mode := strings.TrimSpace(r.config.Mode)
	if mode == "" {
		mode = "per-chat"
	}

	for _, rule := range r.config.Rules {
		if !strings.EqualFold(strings.TrimSpace(rule.Channel), req.Channel) {
			continue
		}
		if rule.Match != "" && !strings.Contains(req.Source, rule.Match) && !strings.Contains(req.Text, rule.Match) {
			continue
		}

		decision := buildDecision(req, defaultString(rule.SessionMode, mode), rule.TitlePrefix)
		if strings.TrimSpace(rule.SessionID) != "" {
			decision.ForcedSessionID = strings.TrimSpace(rule.SessionID)
		}
		decision.QueueMode = strings.TrimSpace(rule.QueueMode)
		if rule.ReplyBack != nil {
			decision.ReplyBack = *rule.ReplyBack
		}
		decision.MatchedRule = fmt.Sprintf("%s:%s", rule.Channel, rule.Match)
		return decision
	}

	return buildDecision(req, mode, "")
}

func buildDecision(req RouteRequest, mode string, titlePrefix string) RouteDecision {
	decision := RouteDecision{SessionMode: mode}
	switch mode {
	case "shared":
		decision.RouteKey = req.Channel + ":shared"
	case "per-message":
		decision.RouteKey = fmt.Sprintf("%s:%s:%d", req.Channel, req.Source, len(req.Text))
	default:
		decision.SessionMode = "per-chat"
		decision.RouteKey = req.Channel + ":" + req.Source
	}

	if strings.TrimSpace(req.ThreadID) != "" {
		decision.RouteKey = decision.RouteKey + ":thread:" + req.ThreadID
		decision.ThreadID = req.ThreadID
	}

	if title := strings.TrimSpace(req.TitleHint); title != "" {
		decision.TitleHint = title
		return decision
	}

	baseTitle := req.Channel + " " + req.Source
	if strings.TrimSpace(req.ThreadID) != "" {
		baseTitle = baseTitle + " (thread)"
	}
	if strings.TrimSpace(titlePrefix) != "" {
		baseTitle = strings.TrimSpace(titlePrefix) + " " + req.Source
	}
	decision.TitleHint = strings.TrimSpace(baseTitle)
	return decision
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
