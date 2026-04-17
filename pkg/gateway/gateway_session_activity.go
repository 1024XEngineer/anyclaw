package gateway

import (
	"github.com/anyclaw/anyclaw/pkg/capability/agents"
	"github.com/anyclaw/anyclaw/pkg/state"
)

func (s *Server) recordSessionToolActivities(session *state.Session, activities []agent.ToolActivity) {
	runner := s.ensureSessionRunner()
	if runner == nil {
		return
	}
	runner.RecordToolActivities(session, activities)
}
