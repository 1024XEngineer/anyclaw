package channel

import (
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type RouteRequest struct {
	Channel  string
	Source   string
	Text     string
	ThreadID string
	IsGroup  bool
	GroupID  string
}

type RouteDecision struct {
	Key         string `json:"key"`
	SessionMode string `json:"session_mode"`
	SessionID   string `json:"session_id,omitempty"`
	QueueMode   string `json:"queue_mode,omitempty"`
	ReplyBack   bool   `json:"reply_back,omitempty"`
	Title       string `json:"title,omitempty"`
	MatchedRule string `json:"matched_rule,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Org         string `json:"org,omitempty"`
	Project     string `json:"project,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	IsThread    bool   `json:"is_thread,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
}

type Router struct {
	config config.RoutingConfig
}

func NewRouter(cfg config.RoutingConfig) *Router {
	return &Router{config: cfg}
}

func (r *Router) Decide(req RouteRequest) RouteDecision {
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
			decision.SessionID = strings.TrimSpace(rule.SessionID)
		}
		decision.QueueMode = strings.TrimSpace(rule.QueueMode)
		if rule.ReplyBack != nil {
			decision.ReplyBack = *rule.ReplyBack
		}
		decision.Agent = strings.TrimSpace(rule.Agent)
		decision.Org = strings.TrimSpace(rule.Org)
		decision.Project = strings.TrimSpace(rule.Project)
		decision.Workspace = defaultString(rule.WorkspaceRef, rule.Workspace)
		decision.MatchedRule = fmt.Sprintf("%s:%s", rule.Channel, rule.Match)
		return decision
	}

	return buildDecision(req, mode, "")
}

func buildDecision(req RouteRequest, mode string, titlePrefix string) RouteDecision {
	decision := RouteDecision{SessionMode: mode}
	switch mode {
	case "shared":
		decision.Key = req.Channel + ":shared"
	case "per-message":
		decision.Key = fmt.Sprintf("%s:%s:%d", req.Channel, req.Source, len(req.Text))
	default:
		decision.SessionMode = "per-chat"
		decision.Key = req.Channel + ":" + req.Source
	}
	if strings.TrimSpace(req.ThreadID) != "" {
		decision.Key = decision.Key + ":thread:" + req.ThreadID
		decision.IsThread = true
		decision.ThreadID = req.ThreadID
	}
	baseTitle := req.Channel + " " + req.Source
	if strings.TrimSpace(req.ThreadID) != "" {
		baseTitle = baseTitle + " (thread)"
	}
	if strings.TrimSpace(titlePrefix) != "" {
		baseTitle = strings.TrimSpace(titlePrefix) + " " + req.Source
	}
	decision.Title = strings.TrimSpace(baseTitle)
	return decision
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
