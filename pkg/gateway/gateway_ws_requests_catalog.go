package gateway

import (
	"context"
	"fmt"
)

func (c *openClawWSConn) handleCatalogWSRequest(ctx context.Context, frame openClawWSFrame, method string) (bool, error) {
	switch method {
	case "agents.list":
		if err := c.requireConfigRead(); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.listAgentViews(), "")
	case "agents.get":
		if err := c.requireConfigRead(); err != nil {
			return true, err
		}
		name := mapString(frame.Params, "name")
		if name == "" {
			return true, fmt.Errorf("name parameter required")
		}
		agent, ok := c.server.getAgentView(name)
		if !ok {
			return true, fmt.Errorf("agent not found: %s", name)
		}
		return true, c.writeResponse(frame.ID, true, agent, "")
	case "providers.list":
		if err := c.requireConfigRead(); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.listProviderViews(), "")
	case "agent-bindings.list", "agent_bindings.list":
		if err := c.requireConfigRead(); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.listAgentBindingViews(), "")
	case "channels.list", "channels.status":
		if err := c.requirePermission("channels.read"); err != nil {
			return true, err
		}
		if c.server.channels == nil {
			return true, c.writeResponse(frame.ID, true, []any{}, "")
		}
		return true, c.writeResponse(frame.ID, true, c.server.channels.Statuses(), "")
	case "sessions.list":
		if err := c.requirePermission("sessions.read"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.filteredSessions(frame.Params), "")
	case "tasks.list":
		if err := c.requirePermission("tasks.read"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.filteredTasks(frame.Params), "")
	case "tools.list", "tools.catalog":
		if err := c.requirePermission("tools.read"); err != nil {
			return true, err
		}
		if c.server.mainRuntime == nil {
			return true, c.writeResponse(frame.ID, true, []any{}, "")
		}
		return true, c.writeResponse(frame.ID, true, c.server.mainRuntime.ListTools(), "")
	case "plugins.list":
		if err := c.requirePermission("plugins.read"); err != nil {
			return true, err
		}
		if c.server.plugins == nil {
			return true, c.writeResponse(frame.ID, true, []any{}, "")
		}
		return true, c.writeResponse(frame.ID, true, c.server.plugins.List(), "")
	default:
		return false, nil
	}
}
