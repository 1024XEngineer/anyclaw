package gateway

import (
	"context"
	"net/http"

	gatewaytransport "github.com/1024XEngineer/anyclaw/pkg/gateway/transport"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) sessionAPI() gatewaytransport.SessionAPI {
	return gatewaytransport.SessionAPI{
		Sessions: s.sessions,
		NormalizeEntryAgent: func(name string) (string, error) {
			return s.mainEntryPolicy().NormalizeAgent(name)
		},
		ResolveSelection: func(r *http.Request) (*state.Org, *state.Project, *state.Workspace, error) {
			orgID, projectID, workspaceID := s.resolveResourceSelection(r)
			return s.validateResourceSelection(orgID, projectID, workspaceID)
		},
		AppendEvent: s.appendEvent,
		WriteJSON:   writeJSON,
	}
}

func (s *Server) sessionMoveAPI() gatewaytransport.SessionMoveAPI {
	return gatewaytransport.SessionMoveAPI{
		Sessions: s.sessions,
		ResolveSelection: func(orgID string, projectID string, workspaceID string) (*state.Org, *state.Project, *state.Workspace, error) {
			return s.validateResourceSelection(orgID, projectID, workspaceID)
		},
		RuntimePool: s.runtimePool,
		AppendAudit: func(ctx context.Context, action string, target string, meta map[string]any) {
			s.appendAudit(UserFromContext(ctx), action, target, meta)
		},
		Store: s.store,
		EnqueueJob: func(job func()) {
			s.jobQueue <- job
		},
		ShouldCancelJob: func(jobID string) bool {
			return s.shouldCancelJob(jobID)
		},
		NextID:         uniqueID,
		JobMaxAttempts: s.jobMaxAttempts,
		WriteJSON:      writeJSON,
	}
}
