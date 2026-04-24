package gateway

import (
	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) recordSessionToolActivities(session *state.Session, activities []agent.ToolActivity) {
	runner := s.ensureSessionRunner()
	if runner == nil {
		return
	}
	runner.RecordToolActivities(session, activities)
}
