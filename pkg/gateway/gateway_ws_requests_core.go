package gateway

import (
	"context"
	"fmt"
	"time"
)

func (c *openClawWSConn) handleCoreWSRequest(ctx context.Context, frame openClawWSFrame, method string) (bool, error) {
	switch method {
	case "connect":
		return true, c.handleWSConnect(frame)
	case "ping":
		return true, c.writeResponse(frame.ID, true, map[string]any{"pong": time.Now().UTC().Format(time.RFC3339)}, "")
	case "methods.list":
		return true, c.writeResponse(frame.ID, true, map[string]any{"methods": openClawWSMethods}, "")
	case "status", "status.get":
		if err := c.requirePermission("status.read"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.status(), "")
	case "control-plane.get", "control_plane.get":
		if err := c.requirePermission("status.read"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.runtimeGovernanceAPI().Snapshot(), "")
	case "events.list":
		if err := c.requirePermission("events.read"); err != nil {
			return true, err
		}
		limit := mapInt(frame.Params, "limit", 24)
		return true, c.writeResponse(frame.ID, true, c.server.store.ListEvents(limit), "")
	case "events.subscribe":
		if err := c.requirePermission("events.read"); err != nil {
			return true, err
		}
		c.startEventStream()
		return true, c.writeResponse(frame.ID, true, map[string]any{"subscribed": true}, "")
	case "events.unsubscribe":
		if err := c.requirePermission("events.read"); err != nil {
			return true, err
		}
		c.stopEventStream()
		return true, c.writeResponse(frame.ID, true, map[string]any{"subscribed": false}, "")
	case "chat.send":
		if err := c.requirePermission("chat.send"); err != nil {
			return true, err
		}
		result, err := c.server.wsChatSend(ctx, c.user, frame.Params)
		if err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, result, "")
	default:
		return false, nil
	}
}

func (c *openClawWSConn) handleWSConnect(frame openClawWSFrame) error {
	provided := firstNonEmpty(mapString(frame.Params, "challenge"), mapString(frame.Params, "nonce"))
	if provided == "" || provided != c.challenge {
		return c.writeResponse(frame.ID, false, nil, "challenge verification failed")
	}
	connectedAt := time.Now().UTC()
	c.connMu.Lock()
	c.connected = true
	c.connectedAt = connectedAt
	c.connMu.Unlock()
	return c.writeResponse(frame.ID, true, map[string]any{
		"status":       "connected",
		"protocol":     "openclaw.gateway.v1",
		"connected_at": connectedAt.Format(time.RFC3339),
		"user":         c.userSummary(),
		"methods":      openClawWSMethods,
	}, "")
}

func (c *openClawWSConn) requireConfigRead() error {
	if HasPermission(c.user, "config.read") || HasPermission(c.user, "config.write") {
		return nil
	}
	return fmt.Errorf("forbidden: missing config.read")
}
