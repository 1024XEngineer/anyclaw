package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type sessionRunOptions struct {
	Source string
	Resume bool
}

func (s *Server) sessionToolApprovalHook(session *Session, cfg *config.Config, title string, message string, source string) agent.ToolApprovalHook {
	if s == nil || s.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous || session == nil {
		return nil
	}
	return func(ctx context.Context, tc agent.ToolCall) error {
		return s.requireSessionToolApproval(session, title, message, source, tc.Name, tc.Args)
	}
}

func (s *Server) sessionProtocolApprovalHook(session *Session, cfg *config.Config, title string, message string, source string) tools.ToolApprovalHook {
	if s == nil || s.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous || session == nil {
		return nil
	}
	return func(ctx context.Context, call tools.ToolApprovalCall) error {
		return s.requireSessionToolApproval(session, title, message, source, call.Name, call.Args)
	}
}

func (s *Server) requireSessionToolApproval(session *Session, title string, message string, source string, toolName string, args map[string]any) error {
	if s == nil || session == nil {
		return nil
	}
	if !requiresToolApprovalName(toolName) {
		return nil
	}
	payload := map[string]any{
		"tool_name":  toolName,
		"args":       cloneAnyMap(args),
		"session_id": session.ID,
		"workspace":  session.Workspace,
		"message":    strings.TrimSpace(message),
		"title":      strings.TrimSpace(title),
	}
	signature := approvalSignature(toolName, "tool_call", payload)
	for _, approval := range s.store.ListSessionApprovals(session.ID) {
		if approval.Signature != signature || approval.ToolName != toolName || approval.Action != "tool_call" {
			continue
		}
		switch approval.Status {
		case "approved":
			return nil
		case "rejected":
			return fmt.Errorf("tool call rejected: %s", toolName)
		case "pending":
			s.updateSessionApprovalPresence(session.ID, toolName)
			return ErrTaskWaitingApproval
		}
	}
	approval, err := s.approvals.Request("", session.ID, 0, toolName, "tool_call", payload)
	if err != nil {
		return err
	}
	s.updateSessionApprovalPresence(session.ID, toolName)
	s.appendEvent("approval.requested", session.ID, map[string]any{
		"approval_id": approval.ID,
		"tool_name":   toolName,
		"action":      "tool_call",
		"status":      approval.Status,
		"source":      firstNonEmpty(strings.TrimSpace(source), "session"),
	})
	return ErrTaskWaitingApproval
}

func (s *Server) updateSessionApprovalPresence(sessionID string, toolName string) {
	if s == nil {
		return
	}
	s.updateSessionPresence(sessionID, "waiting_approval", false)
	s.appendEvent("session.presence", sessionID, map[string]any{
		"presence":  "waiting_approval",
		"tool_name": strings.TrimSpace(toolName),
		"source":    "approval",
	})
}

func (s *Server) updateSessionPresence(sessionID string, presence string, typing bool) {
	if s == nil || s.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	_, _ = s.sessions.SetPresence(sessionID, presence, typing)
}

func (s *Server) sessionApprovalResponse(sessionID string) map[string]any {
	response := map[string]any{
		"status":    "waiting_approval",
		"approvals": s.store.ListSessionApprovals(sessionID),
	}
	if session, ok := s.sessions.Get(sessionID); ok {
		response["session"] = session
	}
	return response
}

func (s *Server) resumeApprovedSessionApproval(ctx context.Context, approval *Approval) error {
	if s == nil || approval == nil {
		return nil
	}
	sessionID := strings.TrimSpace(approval.SessionID)
	if sessionID == "" {
		return nil
	}
	message, _ := approval.Payload["message"].(string)
	title, _ := approval.Payload["title"].(string)
	message = strings.TrimSpace(message)
	title = strings.TrimSpace(title)
	if message == "" {
		return nil
	}
	_, _, err := s.runSessionMessageWithOptions(ctx, sessionID, title, message, sessionRunOptions{
		Source: "approval_resume",
		Resume: true,
	})
	if err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			return err
		}
		s.updateSessionPresence(sessionID, "idle", false)
		s.appendEvent("chat.failed", sessionID, map[string]any{
			"message": message,
			"error":   err.Error(),
			"source":  "approval_resume",
		})
		return err
	}
	return nil
}
