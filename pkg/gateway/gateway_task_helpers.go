package gateway

import (
	"context"
	"net/http"
	"strings"

	gatewaytransport "github.com/1024XEngineer/anyclaw/pkg/gateway/transport"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) taskAPI() gatewaytransport.TaskAPI {
	return gatewaytransport.TaskAPI{
		Tasks:    s.tasks,
		Sessions: s.sessions,
		CheckPermission: func(ctx context.Context, permission string) bool {
			return HasPermission(UserFromContext(ctx), permission)
		},
		AppendAudit: func(ctx context.Context, action string, target string, meta map[string]any) {
			s.appendAudit(UserFromContext(ctx), action, target, meta)
		},
		NormalizeEntryAgent: func(name string) (string, error) {
			return s.mainEntryPolicy().NormalizeAgent(name)
		},
		ResolveSelection: func(r *http.Request) (*state.Org, *state.Project, *state.Workspace, error) {
			orgID, projectID, workspaceID := s.resolveHierarchyFromQuery(r)
			return s.validateResourceSelection(orgID, projectID, workspaceID)
		},
		BuildResponse:    s.taskResponse,
		RecordCompletion: s.recordTaskCompletion,
		WriteJSON:        writeJSON,
	}
}

func (s *Server) taskResponse(task *state.Task, session *state.Session) map[string]any {
	response := map[string]any{
		"task":      task,
		"steps":     s.tasks.Steps(task.ID),
		"approvals": s.store.ListTaskApprovals(task.ID),
	}
	if session != nil {
		response["session"] = session
	} else if strings.TrimSpace(task.SessionID) != "" {
		if linkedSession, ok := s.sessions.Get(task.SessionID); ok {
			response["session"] = linkedSession
		}
	}
	return response
}

func (s *Server) recordTaskCompletion(result *taskrunner.ExecutionResult, source string) {
	if result == nil || result.Task == nil || result.Session == nil {
		return
	}
	s.appendEvent("task.completed", result.Session.ID, map[string]any{"task_id": result.Task.ID, "status": result.Task.Status, "source": source})
	freshSession, ok := s.sessions.Get(result.Session.ID)
	if !ok {
		return
	}
	s.recordSessionToolActivities(freshSession, result.ToolActivities)
}
