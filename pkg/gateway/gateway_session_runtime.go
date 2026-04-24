package gateway

import (
	"context"
	"net/http"

	gatewayapprovals "github.com/1024XEngineer/anyclaw/pkg/gateway/approvals"
	sessionrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) approvalsAPI() gatewayapprovals.API {
	return gatewayapprovals.API{
		Store:       s.store,
		Approvals:   s.approvals,
		CurrentUser: UserFromContext,
		AppendAudit: s.appendAudit,
		AppendEvent: s.appendEvent,
		OnResolved: func(updated *state.Approval, approved bool, comment string) {
			s.approvalsService().HandleResolved(updated, approved, comment)
		},
	}
}

func (s *Server) approvalsService() gatewayapprovals.Service {
	return gatewayapprovals.Service{
		Store:                s.store,
		Sessions:             s.sessions,
		Tasks:                s.tasks,
		Runner:               s.ensureSessionRunner(),
		AppendEvent:          s.appendEvent,
		RecordTaskCompletion: s.recordTaskCompletion,
	}
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	s.approvalsAPI().HandleList(w, r)
}

func (s *Server) handleApprovalByID(w http.ResponseWriter, r *http.Request) {
	s.approvalsAPI().HandleByID(w, r)
}

func (s *Server) handleResolvedApproval(updated *state.Approval, approved bool, comment string) {
	s.approvalsService().HandleResolved(updated, approved, comment)
}

func (s *Server) requireSessionToolApproval(session *state.Session, title string, message string, source string, toolName string, args map[string]any) error {
	return s.approvalsService().RequireSessionToolApproval(session, title, message, source, toolName, args)
}

func (s *Server) updateSessionApprovalPresence(sessionID string, toolName string) {
	s.approvalsService().UpdateSessionApprovalPresence(sessionID, toolName)
}

func (s *Server) updateSessionPresence(sessionID string, presence string, typing bool) {
	s.approvalsService().UpdateSessionPresence(sessionID, presence, typing)
}

func (s *Server) sessionApprovalResponse(sessionID string) map[string]any {
	return s.approvalsService().SessionApprovalResponse(sessionID)
}

func (s *Server) resumeApprovedSessionApproval(ctx context.Context, approval *state.Approval) error {
	return s.approvalsService().ResumeApprovedSessionApproval(ctx, approval)
}

func (s *Server) runSessionMessage(ctx context.Context, sessionID string, title string, message string) (string, *state.Session, error) {
	return s.runSessionMessageWithOptions(ctx, sessionID, title, message, sessionrunner.RunOptions{Source: "api", Channel: "api"})
}

func (s *Server) runSessionMessageWithOptions(ctx context.Context, sessionID string, title string, message string, opts sessionrunner.RunOptions) (string, *state.Session, error) {
	return s.sessionBridgeService().RunMessage(ctx, sessionID, title, message, opts)
}
