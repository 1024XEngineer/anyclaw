package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	gatewayevents "github.com/1024XEngineer/anyclaw/pkg/gateway/events"
	gatewayintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type SessionAgentResolver func(name string) (string, error)
type SessionSelectionResolver func(r *http.Request) (*state.Org, *state.Project, *state.Workspace, error)
type SessionEventRecorder func(eventType string, sessionID string, payload map[string]any)
type SessionJSONWriter func(w http.ResponseWriter, statusCode int, value any)

type SessionAPI struct {
	Sessions            *state.SessionManager
	NormalizeEntryAgent SessionAgentResolver
	ResolveSelection    SessionSelectionResolver
	AppendEvent         SessionEventRecorder
	WriteJSON           SessionJSONWriter
}

func (api SessionAPI) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.handleList(w, r)
	case http.MethodPost:
		api.handleCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api SessionAPI) HandleByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.handleGetByID(w, r)
	case http.MethodDelete:
		api.handleDeleteByID(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api SessionAPI) handleGetByID(w http.ResponseWriter, r *http.Request) {
	if api.Sessions == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session manager not available"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	session, ok := api.Sessions.Get(id)
	if !ok {
		api.writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	api.writeJSON(w, http.StatusOK, session)
}

func (api SessionAPI) handleDeleteByID(w http.ResponseWriter, r *http.Request) {
	if api.Sessions == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session manager not available"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	if err := api.Sessions.Delete(id); err != nil {
		if strings.Contains(err.Error(), "session not found") {
			api.writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	api.appendEvent("session.deleted", id, map[string]any{"id": id})
	api.writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

func (api SessionAPI) handleList(w http.ResponseWriter, r *http.Request) {
	if api.Sessions == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session manager not available"})
		return
	}

	workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
	sessions := api.Sessions.List()
	if workspace != "" {
		filtered := make([]*state.Session, 0, len(sessions))
		for _, session := range sessions {
			if state.SessionExecutionWorkspace(session) == workspace {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}
	api.writeJSON(w, http.StatusOK, sessions)
}

func (api SessionAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
	if api.Sessions == nil {
		api.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session manager not available"})
		return
	}

	var req struct {
		Title        string   `json:"title"`
		Agent        string   `json:"agent"`
		Assistant    string   `json:"assistant"`
		Participants []string `json:"participants"`
		SessionMode  string   `json:"session_mode"`
		QueueMode    string   `json:"queue_mode"`
		ReplyBack    bool     `json:"reply_back"`
		IsGroup      bool     `json:"is_group"`
		GroupKey     string   `json:"group_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, context.Canceled) {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	agentName, err := api.normalizeEntryAgent(gatewayintake.RequestedAgentName(req.Agent, req.Assistant))
	if err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	resolvedParticipants := make([]string, 0, len(req.Participants))
	seenParticipants := map[string]bool{}
	for _, name := range req.Participants {
		resolvedName, err := api.normalizeEntryAgent(name)
		if err != nil {
			api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if resolvedName == "" || seenParticipants[resolvedName] {
			continue
		}
		seenParticipants[resolvedName] = true
		resolvedParticipants = append(resolvedParticipants, resolvedName)
	}
	resolvedParticipants = state.NormalizeParticipants(agentName, resolvedParticipants)
	if req.IsGroup || strings.TrimSpace(req.GroupKey) != "" || gatewayintake.IsGroupSessionMode(req.SessionMode) || len(resolvedParticipants) > 1 {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "multi-agent session creation is not supported on /sessions; use single-agent sessions only",
		})
		return
	}

	org, project, workspace, err := api.resolveSelection(r)
	if err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	session, err := api.Sessions.CreateWithOptions(state.SessionCreateOptions{
		Title:       req.Title,
		AgentName:   agentName,
		Org:         org.ID,
		Project:     project.ID,
		Workspace:   workspace.ID,
		SessionMode: gatewayintake.NormalizeSingleAgentSessionMode(req.SessionMode, "main"),
		QueueMode:   req.QueueMode,
		ReplyBack:   req.ReplyBack,
	})
	if err != nil {
		api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	api.appendEvent("session.created", session.ID, gatewayevents.SessionCreatedEventPayload(session))
	api.writeJSON(w, http.StatusCreated, session)
}

func (api SessionAPI) normalizeEntryAgent(name string) (string, error) {
	if api.NormalizeEntryAgent == nil {
		return strings.TrimSpace(name), nil
	}
	return api.NormalizeEntryAgent(name)
}

func (api SessionAPI) resolveSelection(r *http.Request) (*state.Org, *state.Project, *state.Workspace, error) {
	if api.ResolveSelection == nil {
		return &state.Org{}, &state.Project{}, &state.Workspace{}, nil
	}
	return api.ResolveSelection(r)
}

func (api SessionAPI) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if api.AppendEvent == nil {
		return
	}
	api.AppendEvent(eventType, sessionID, payload)
}

func (api SessionAPI) writeJSON(w http.ResponseWriter, statusCode int, value any) {
	if api.WriteJSON != nil {
		api.WriteJSON(w, statusCode, value)
		return
	}
	writeJSON(w, statusCode, value)
}
