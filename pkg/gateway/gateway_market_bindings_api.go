package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	"github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func (s *Server) handleMarketBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.marketplaceStore().ListBindings()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": result})
	case http.MethodPost:
		var req marketplace.BindingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		normalized, err := s.normalizeMarketBindingRequest(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		binding, err := s.marketplaceStore().CreateBinding(normalized)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.appendMarketAudit(marketplace.MarketAuditEvent{
			Type:       "market.binding.created",
			ArtifactID: binding.ArtifactID,
			BindingID:  binding.ID,
			Actor:      marketActor(r),
			Detail: map[string]any{
				"target_type": binding.TargetType,
				"target_id":   binding.TargetID,
				"version":     binding.Version,
			},
		})
		s.appendMarketEvent(marketplace.MarketEvent{
			Type:       "market.binding.created",
			Level:      "success",
			Message:    "Marketplace binding created",
			ArtifactID: binding.ArtifactID,
			BindingID:  binding.ID,
			Payload: map[string]any{
				"target_type": binding.TargetType,
				"target_id":   binding.TargetID,
			},
		})
		s.refreshMarketBinding(binding)
		writeJSON(w, http.StatusOK, map[string]any{"data": binding})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMarketBindingByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/market/bindings/"), "/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "binding id required"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.marketplaceStore().DeleteBinding(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "binding not found"})
			return
		}
		s.appendMarketAudit(marketplace.MarketAuditEvent{
			Type:      "market.binding.deleted",
			BindingID: id,
			Actor:     marketActor(r),
		})
		s.appendMarketEvent(marketplace.MarketEvent{
			Type:      "market.binding.deleted",
			Level:     "info",
			Message:   "Marketplace binding deleted",
			BindingID: id,
		})
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMarketEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.marketplaceStore().ListEvents(parseIntParam(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) appendMarketAudit(event marketplace.MarketAuditEvent) {
	if s == nil {
		return
	}
	_ = s.marketplaceStore().AppendAudit(event)
}

func (s *Server) appendMarketEvent(event marketplace.MarketEvent) {
	if s == nil {
		return
	}
	_ = s.marketplaceStore().AppendEvent(event)
}

func marketActor(r *http.Request) string {
	if user := UserFromContext(r.Context()); user != nil && strings.TrimSpace(user.Name) != "" {
		return user.Name
	}
	return "user"
}

func (s *Server) handleMarketRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		BindingID string `json:"binding_id,omitempty"`
		Scope     string `json:"scope,omitempty"`
		Agent     string `json:"agent,omitempty"`
		Org       string `json:"org,omitempty"`
		Project   string `json:"project,omitempty"`
		Workspace string `json:"workspace,omitempty"`
		SessionID string `json:"session_id,omitempty"`
		Warm      bool   `json:"warm,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	scopeKind := runtime.RefreshScopeKind(strings.TrimSpace(req.Scope))
	if scopeKind == "" && strings.TrimSpace(req.SessionID) != "" {
		scopeKind = runtime.RefreshScopeSession
	}
	agent, orgID, projectID, workspaceID := req.Agent, req.Org, req.Project, req.Workspace
	if scopeKind != runtime.RefreshScopeSession {
		agent, orgID, projectID, workspaceID = s.marketRefreshTarget(req.Agent, req.Org, req.Project, req.Workspace)
	}
	result := s.marketHotReload().Refresh(r.Context(), runtime.RefreshScope{
		Kind:      scopeKind,
		Agent:     agent,
		Org:       orgID,
		Project:   projectID,
		Workspace: workspaceID,
		SessionID: req.SessionID,
		Reason:    "market.manual_refresh",
		Warm:      req.Warm,
	})
	if result.Status == "failed" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": "failed",
			"error":  result.Error,
			"result": result,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "refreshed",
		"agent":     agent,
		"org":       orgID,
		"project":   projectID,
		"workspace": workspaceID,
		"result":    result,
	})
}

func (s *Server) normalizeMarketBindingRequest(req marketplace.BindingRequest) (marketplace.BindingRequest, error) {
	req.ArtifactID = strings.TrimSpace(req.ArtifactID)
	req.ReceiptID = strings.TrimSpace(req.ReceiptID)
	req.TargetType = marketplace.NormalizeBindingTargetType(string(req.TargetType))
	req.TargetID = strings.TrimSpace(req.TargetID)
	if req.TargetType == "" {
		return req, errString("invalid target_type")
	}
	if req.TargetType == marketplace.TargetMainAgent {
		req.TargetID = strings.TrimSpace(s.mainRuntime.Config.ResolveMainAgentName())
		if req.TargetID == "" {
			req.TargetID = strings.TrimSpace(s.mainRuntime.Config.Agent.Name)
		}
	}
	if req.TargetType == marketplace.TargetWorkspace && req.TargetID == "" {
		_, _, workspaceID := defaultResourceIDs(s.mainRuntime.WorkingDir)
		req.TargetID = workspaceID
	}
	return req, nil
}

func (s *Server) refreshMarketBinding(binding *marketplace.Binding) {
	if s == nil || binding == nil {
		return
	}
	agent, orgID, projectID, workspaceID := s.marketRefreshTarget("", "", "", "")
	scope := runtime.RefreshScope{
		Kind:      runtime.RefreshScopeRuntime,
		Agent:     agent,
		Org:       orgID,
		Project:   projectID,
		Workspace: workspaceID,
		Reason:    "market.binding",
	}
	switch binding.TargetType {
	case marketplace.TargetMainAgent:
		scope.Kind = runtime.RefreshScopeAgent
		scope.Agent = binding.TargetID
	case marketplace.TargetWorkspace:
		scope.Kind = runtime.RefreshScopeWorkspace
		scope.Workspace = binding.TargetID
	case marketplace.TargetRuntimeGlobal:
		scope.Kind = runtime.RefreshScopeGlobal
	case marketplace.TargetPersistentSubagent:
		scope.Kind = runtime.RefreshScopeAgent
		scope.Agent = binding.TargetID
	}
	result := s.marketHotReload().Refresh(context.Background(), scope)
	if result.Status == "failed" {
		s.appendMarketEvent(marketplace.MarketEvent{
			Type:       "market.refresh.failed",
			Level:      "error",
			Message:    result.Error,
			ArtifactID: binding.ArtifactID,
			BindingID:  binding.ID,
			Payload: map[string]any{
				"scope": result.Scope,
				"error": result.Error,
			},
		})
	}
}

func (s *Server) refreshMarketArtifactBindings(artifactID string) {
	if s == nil || strings.TrimSpace(artifactID) == "" {
		return
	}
	result, err := s.marketplaceStore().ListBindings()
	if err != nil {
		s.appendMarketEvent(marketplace.MarketEvent{
			Type:       "market.refresh.failed",
			Level:      "error",
			Message:    err.Error(),
			ArtifactID: artifactID,
		})
		return
	}
	for i := range result.Items {
		binding := result.Items[i]
		if !strings.EqualFold(strings.TrimSpace(binding.ArtifactID), strings.TrimSpace(artifactID)) {
			continue
		}
		if binding.State != marketplace.BindingEnabled {
			continue
		}
		s.refreshMarketBinding(&binding)
	}
}

func (s *Server) marketRefreshTarget(agent, orgID, projectID, workspaceID string) (string, string, string, string) {
	if s == nil || s.mainRuntime == nil {
		return agent, orgID, projectID, workspaceID
	}
	if strings.TrimSpace(agent) == "" && s.mainRuntime.Config != nil {
		agent = strings.TrimSpace(s.mainRuntime.Config.ResolveMainAgentName())
		if agent == "" {
			agent = strings.TrimSpace(s.mainRuntime.Config.Agent.Name)
		}
	}
	defaultOrg, defaultProject, defaultWorkspace := defaultResourceIDs(s.mainRuntime.WorkingDir)
	if strings.TrimSpace(orgID) == "" {
		orgID = defaultOrg
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = defaultProject
	}
	if strings.TrimSpace(workspaceID) == "" {
		workspaceID = defaultWorkspace
	}
	return agent, orgID, projectID, workspaceID
}

type errString string

func (e errString) Error() string {
	return string(e)
}

func (s *Server) marketHotReload() *runtime.HotReloadCoordinator {
	if s == nil {
		return nil
	}
	if s.hotReload == nil {
		s.hotReload = runtime.NewHotReloadCoordinator(s.runtimePool, s.store)
	}
	return s.hotReload
}
