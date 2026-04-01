package gateway

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/plugin"
)

type appWorkflowMatchView struct {
	Plugin      string                       `json:"plugin"`
	App         string                       `json:"app"`
	Name        string                       `json:"name"`
	Description string                       `json:"description,omitempty"`
	Action      string                       `json:"action"`
	ToolName    string                       `json:"tool_name"`
	Tags        []string                     `json:"tags,omitempty"`
	Score       int                          `json:"score"`
	Reason      string                       `json:"reason,omitempty"`
	Pairing     *appWorkflowPairingMatchView `json:"pairing,omitempty"`
}

type appWorkflowPairingMatchView struct {
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Binding     string         `json:"binding,omitempty"`
	Triggers    []string       `json:"triggers,omitempty"`
	Defaults    map[string]any `json:"defaults,omitempty"`
}

type appWorkflowResolveResponse struct {
	Query   string                 `json:"query"`
	Matches []appWorkflowMatchView `json:"matches"`
}

func workflowMatchToView(match plugin.AppWorkflowMatch) appWorkflowMatchView {
	return appWorkflowMatchView{
		Plugin:      strings.TrimSpace(match.Workflow.Plugin),
		App:         strings.TrimSpace(match.Workflow.App),
		Name:        strings.TrimSpace(match.Workflow.Name),
		Description: strings.TrimSpace(match.Workflow.Description),
		Action:      strings.TrimSpace(match.Workflow.Action),
		ToolName:    strings.TrimSpace(match.Workflow.ToolName),
		Tags:        append([]string{}, match.Workflow.Tags...),
		Score:       match.Score,
		Reason:      strings.TrimSpace(match.Reason),
		Pairing:     workflowPairingToView(match.Pairing),
	}
}

func workflowPairingToView(pairing *plugin.AppWorkflowPairingInfo) *appWorkflowPairingMatchView {
	if pairing == nil {
		return nil
	}
	return &appWorkflowPairingMatchView{
		ID:          strings.TrimSpace(pairing.ID),
		Name:        strings.TrimSpace(pairing.Name),
		Description: strings.TrimSpace(pairing.Description),
		Binding:     strings.TrimSpace(pairing.Binding),
		Triggers:    append([]string{}, pairing.Triggers...),
		Defaults:    cloneAnyValues(pairing.Defaults),
	}
}

func (s *Server) resolveAppWorkflowViews(ctx context.Context, query string, limit int) appWorkflowResolveResponse {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}
	if s.plugins == nil {
		return appWorkflowResolveResponse{Query: query, Matches: []appWorkflowMatchView{}}
	}

	matches := s.plugins.ResolveWorkflowMatches(query, limit)
	if s.app != nil && strings.TrimSpace(s.app.ConfigPath) != "" {
		if store, err := newAppStore(s.app.ConfigPath); err == nil {
			matches = s.plugins.ResolveWorkflowMatchesWithPairings(query, limit, store.ListPairings())
		}
	}
	if s.tasks != nil && s.app != nil {
		matches = trimWorkflowMatches(s.tasks.resolveWorkflowMatches(ctx, query, s.plugins, s.app.LLM), limit)
	}
	items := make([]appWorkflowMatchView, 0, len(matches))
	for _, match := range matches {
		items = append(items, workflowMatchToView(match))
	}
	return appWorkflowResolveResponse{Query: query, Matches: items}
}

func (s *Server) handleAppWorkflowResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}

	limit := 3
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		if value > 10 {
			value = 10
		}
		limit = value
	}

	response := s.resolveAppWorkflowViews(r.Context(), query, limit)
	s.appendAudit(UserFromContext(r.Context()), "apps.resolve_workflows", query, map[string]any{"limit": limit, "match_count": len(response.Matches)})
	writeJSON(w, http.StatusOK, response)
}
