package gateway

import sessionrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionrunner"

func (s *Server) ensureSessionRunner() *sessionrunner.Manager {
	if s == nil {
		return nil
	}
	if s.sessionRunner == nil {
		s.sessionRunner = sessionrunner.NewManager(s.store, s.sessions, s.runtimePool, s.approvals, sessionEventRecorder{server: s})
	}
	return s.sessionRunner
}

type sessionEventRecorder struct {
	server *Server
}

func (r sessionEventRecorder) AppendEvent(eventType string, sessionID string, payload map[string]any) {
	if r.server == nil {
		return
	}
	r.server.appendEvent(eventType, sessionID, payload)
}
