package gateway

import (
	"net/http"

	gatewayresources "github.com/1024XEngineer/anyclaw/pkg/gateway/resources"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) resourcesAPI() gatewayresources.API {
	return gatewayresources.API{
		Store:       s.store,
		RuntimePool: s.runtimePool,
		CheckPermission: func(r *http.Request, permission string) bool {
			return HasPermission(UserFromContext(r.Context()), permission)
		},
		AppendAudit: func(r *http.Request, action string, target string, meta map[string]any) {
			s.appendAudit(UserFromContext(r.Context()), action, target, meta)
		},
		WriteJSON: writeJSON,
	}
}

func (s *Server) resolveWorkspaceFromQuery(r *http.Request) string {
	return gatewayresources.ResolveWorkspaceFromQuery(r)
}

func (s *Server) resolveHierarchyFromQuery(r *http.Request) (string, string, string) {
	return gatewayresources.ResolveHierarchyFromQuery(r)
}

func (s *Server) resolveWorkspaceFromSessionPath(r *http.Request) string {
	return gatewayresources.ResolveWorkspaceFromSessionPath(r, s.sessions)
}

func (s *Server) resolveHierarchyFromSessionPath(r *http.Request) (string, string, string) {
	return gatewayresources.ResolveHierarchyFromSessionPath(r, s.sessions)
}

func (s *Server) resolveSessionWorkspaceFromChat(r *http.Request) string {
	return gatewayresources.ResolveSessionWorkspaceFromChat(r, s.sessions)
}

func (s *Server) resolveResourceSelection(r *http.Request) (string, string, string) {
	return gatewayresources.ResolveSelection(r)
}

func (s *Server) validateResourceSelection(orgID string, projectID string, workspaceID string) (*state.Org, *state.Project, *state.Workspace, error) {
	return gatewayresources.ValidateSelection(s.store, orgID, projectID, workspaceID)
}

func defaultResourceIDs(workingDir string) (string, string, string) {
	return gatewayresources.DefaultIDs(workingDir)
}

func (s *Server) ensureDefaultWorkspace() error {
	return gatewayresources.EnsureDefaultWorkspace(s.store, s.mainRuntime.WorkingDir)
}
