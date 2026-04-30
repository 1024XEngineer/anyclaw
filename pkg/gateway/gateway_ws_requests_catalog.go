package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
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
	case "tools.invoke", "tools_invoke":
		if err := c.requirePermission("tools.write"); err != nil {
			return true, err
		}
		result, err := c.invokeToolFromWS(ctx, frame)
		if err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, result, "")
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

func (c *openClawWSConn) invokeToolFromWS(ctx context.Context, frame openClawWSFrame) (string, error) {
	if c == nil || c.server == nil || c.server.mainRuntime == nil {
		return "", fmt.Errorf("runtime tool registry is unavailable")
	}
	toolName := firstNonEmpty(mapString(frame.Params, "tool"), mapString(frame.Params, "name"))
	if toolName == "" {
		return "", fmt.Errorf("tool parameter required")
	}
	args, err := wsToolArgs(frame.Params)
	if err != nil {
		return "", err
	}
	registry := c.server.mainRuntime.ToolRegistry()
	if toolRequiresWSApproval(registry, toolName) {
		return "", fmt.Errorf("tool %s requires approval; invoke it through a task or session", toolName)
	}
	callCtx := tools.WithToolCaller(ctx, tools.ToolCaller{
		Role:        tools.ToolCallerRoleControlAPI,
		AgentName:   mapString(c.userSummary(), "name"),
		ExecutionID: frame.ID,
	})
	return c.server.mainRuntime.CallTool(callCtx, toolName, args)
}

func wsToolArgs(params map[string]any) (map[string]any, error) {
	if params == nil {
		return map[string]any{}, nil
	}
	raw, ok := params["args"]
	if !ok {
		raw = params["input"]
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	switch value := raw.(type) {
	case map[string]any:
		return value, nil
	case json.RawMessage:
		return decodeWSToolArgs(value)
	case []byte:
		return decodeWSToolArgs(value)
	case string:
		if value == "" {
			return map[string]any{}, nil
		}
		return decodeWSToolArgs([]byte(value))
	default:
		return nil, fmt.Errorf("args must be an object")
	}
}

func decodeWSToolArgs(data []byte) (map[string]any, error) {
	var args map[string]any
	if err := json.Unmarshal(data, &args); err != nil {
		return nil, fmt.Errorf("invalid tool args: %w", err)
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func toolRequiresWSApproval(registry *tools.Registry, name string) bool {
	return taskrunner.RequiresToolApprovalName(name) || (registry != nil && registry.RequiresApproval(name))
}
