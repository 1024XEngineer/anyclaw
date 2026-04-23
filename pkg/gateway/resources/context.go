package resources

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func ResolveWorkspaceFromQuery(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("workspace"))
}

func ResolveHierarchyFromQuery(r *http.Request) (string, string, string) {
	return strings.TrimSpace(r.URL.Query().Get("org")), strings.TrimSpace(r.URL.Query().Get("project")), strings.TrimSpace(r.URL.Query().Get("workspace"))
}

func ResolveWorkspaceFromSessionPath(r *http.Request, sessions *state.SessionManager) string {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" || sessions == nil {
		return ""
	}
	session, ok := sessions.Get(id)
	if !ok {
		return ""
	}
	return state.SessionExecutionWorkspace(session)
}

func ResolveHierarchyFromSessionPath(r *http.Request, sessions *state.SessionManager) (string, string, string) {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" || sessions == nil {
		return "", "", ""
	}
	session, ok := sessions.Get(id)
	if !ok {
		return "", "", ""
	}
	return state.SessionExecutionHierarchy(session)
}

func ResolveSessionWorkspaceFromChat(r *http.Request, sessions *state.SessionManager) string {
	if r.Method != http.MethodPost || sessions == nil {
		return ""
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return strings.TrimSpace(r.URL.Query().Get("workspace"))
	}
	session, ok := sessions.Get(strings.TrimSpace(req.SessionID))
	if !ok {
		return ""
	}
	return state.SessionExecutionWorkspace(session)
}

func ResolveSelection(r *http.Request) (string, string, string) {
	org := strings.TrimSpace(r.URL.Query().Get("org"))
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
	return org, project, workspace
}
