package gateway

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (c *openClawWSConn) filteredSessions(params map[string]any) []*state.Session {
	workspace := mapString(params, "workspace")
	items := c.server.store.ListSessions()
	if workspace == "" {
		return items
	}
	filtered := make([]*state.Session, 0, len(items))
	for _, session := range items {
		if state.SessionExecutionWorkspace(session) == workspace {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func (c *openClawWSConn) filteredTasks(params map[string]any) []*state.Task {
	workspace := mapString(params, "workspace")
	status := mapString(params, "status")
	items := c.server.store.ListTasks()
	filtered := make([]*state.Task, 0, len(items))
	for _, task := range items {
		if workspace != "" && task.Workspace != workspace {
			continue
		}
		if status != "" && !strings.EqualFold(task.Status, status) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered
}
