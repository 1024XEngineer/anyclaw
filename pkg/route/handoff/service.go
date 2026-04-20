package handoff

import "strings"

const (
	ModeMain               = "main"
	ModePersistentSubagent = "persistent_subagent"
	ModeTemporarySubagent  = "temporary_subagent"
)

type HandoffRoutingEntry struct {
	SessionID           string
	UserInput           string
	PreferredSubagentID string
	SkipDelegation      bool
	Metadata            map[string]string
}

type HandoffRequest struct {
	SessionID           string
	UserInput           string
	PreferredSubagentID string
	SkipDelegation      bool
}

type PlanOptions struct {
	PersistentFirst bool
	AllowTemporary  bool
}

type HandoffPlan struct {
	Mode          string
	TargetAgentID string
	SessionID     string
	Persistence   string
	Reason        string
}

type PersistentMatcher interface {
	AvailableAgentNames() []string
}

type Router struct {
	persistent PersistentMatcher
}

func NewRouter(persistent PersistentMatcher) *Router {
	return &Router{persistent: persistent}
}

func (r *Router) Prepare(entry HandoffRoutingEntry) HandoffRequest {
	return HandoffRequest{
		SessionID:           strings.TrimSpace(entry.SessionID),
		UserInput:           strings.TrimSpace(entry.UserInput),
		PreferredSubagentID: strings.TrimSpace(entry.PreferredSubagentID),
		SkipDelegation:      entry.SkipDelegation,
	}
}

func (r *Router) Plan(req HandoffRequest, options PlanOptions) HandoffPlan {
	req = HandoffRequest{
		SessionID:           strings.TrimSpace(req.SessionID),
		UserInput:           strings.TrimSpace(req.UserInput),
		PreferredSubagentID: strings.TrimSpace(req.PreferredSubagentID),
		SkipDelegation:      req.SkipDelegation,
	}

	if req.SkipDelegation {
		return HandoffPlan{
			Mode:        ModeMain,
			SessionID:   req.SessionID,
			Persistence: "main",
			Reason:      "skip_delegation requested",
		}
	}

	if req.UserInput == "" {
		return HandoffPlan{
			Mode:        ModeMain,
			SessionID:   req.SessionID,
			Persistence: "main",
			Reason:      "empty user input",
		}
	}

	available := availablePersistentNames(r.persistent)
	if preferred := req.PreferredSubagentID; preferred != "" {
		if resolved, ok := resolvePersistentName(available, preferred); ok {
			return HandoffPlan{
				Mode:          ModePersistentSubagent,
				TargetAgentID: resolved,
				SessionID:     req.SessionID,
				Persistence:   "persistent",
				Reason:        "explicit preferred persistent subagent",
			}
		}
		if options.AllowTemporary {
			return HandoffPlan{
				Mode:          ModeTemporarySubagent,
				TargetAgentID: preferred,
				SessionID:     req.SessionID,
				Persistence:   "temporary",
				Reason:        "preferred persistent subagent unavailable; falling back to temporary",
			}
		}
		return HandoffPlan{
			Mode:        ModeMain,
			SessionID:   req.SessionID,
			Persistence: "main",
			Reason:      "preferred persistent subagent unavailable",
		}
	}

	if len(available) == 1 {
		return HandoffPlan{
			Mode:          ModePersistentSubagent,
			TargetAgentID: available[0],
			SessionID:     req.SessionID,
			Persistence:   "persistent",
			Reason:        "single persistent subagent available",
		}
	}

	if len(available) > 1 {
		if !options.PersistentFirst && options.AllowTemporary {
			return HandoffPlan{
				Mode:        ModeTemporarySubagent,
				SessionID:   req.SessionID,
				Persistence: "temporary",
				Reason:      "multiple persistent subagents available without an explicit preference",
			}
		}
		return HandoffPlan{
			Mode:        ModeMain,
			SessionID:   req.SessionID,
			Persistence: "main",
			Reason:      "multiple persistent subagents available without an explicit preference",
		}
	}

	if options.AllowTemporary {
		return HandoffPlan{
			Mode:        ModeTemporarySubagent,
			SessionID:   req.SessionID,
			Persistence: "temporary",
			Reason:      "no persistent subagent available",
		}
	}

	return HandoffPlan{
		Mode:        ModeMain,
		SessionID:   req.SessionID,
		Persistence: "main",
		Reason:      "no persistent subagent available",
	}
}

func availablePersistentNames(matcher PersistentMatcher) []string {
	if matcher == nil {
		return nil
	}
	return normalizeNames(matcher.AvailableAgentNames())
}

func resolvePersistentName(available []string, preferred string) (string, bool) {
	preferred = strings.TrimSpace(preferred)
	if preferred == "" {
		return "", false
	}
	for _, name := range available {
		if strings.EqualFold(name, preferred) {
			return name, true
		}
	}
	return "", false
}

func normalizeNames(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}
