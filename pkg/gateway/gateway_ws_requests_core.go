package gateway

import (
	"context"
	"fmt"
	"time"

	gatewaygovernance "github.com/1024XEngineer/anyclaw/pkg/gateway/governance"
)

func (c *openClawWSConn) handleCoreWSRequest(ctx context.Context, frame openClawWSFrame, method string) (bool, error) {
	switch method {
	case "connect":
		return true, c.handleWSConnect(frame)
	case "ping":
		return true, c.writeResponse(frame.ID, true, c.server.controlPlaneService().Ping(time.Now()), "")
	case "methods.list":
		return true, c.writeResponse(frame.ID, true, c.server.controlPlaneService().MethodsList(), "")
	case "status", "status.get":
		if err := c.requirePermission("status.read"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.status(), "")
	case "control-plane.get", "control_plane.get":
		if err := c.requirePermission("status.read"); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.controlPlaneRuntimeAPI().Snapshot(), "")
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
		return true, c.writeResponse(frame.ID, true, c.server.controlPlaneService().Subscription(true), "")
	case "events.unsubscribe":
		if err := c.requirePermission("events.read"); err != nil {
			return true, err
		}
		c.stopEventStream()
		return true, c.writeResponse(frame.ID, true, c.server.controlPlaneService().Subscription(false), "")
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
	if _, err := c.server.governanceService().AuthorizeConnection(c.contextWithUser(), gatewaygovernance.ConnectionRequest{
		ConnectionID: c.challenge,
		Protocol:     c.transport.Protocol,
		Path:         c.transport.Path,
		ClientID:     c.transport.ClientID,
		Metadata: map[string]string{
			"method": "connect",
		},
	}); err != nil {
		return c.writeResponse(frame.ID, false, nil, err.Error())
	}
	connectedAt := time.Now().UTC()
	c.connMu.Lock()
	c.connected = true
	c.connectedAt = connectedAt
	c.connMu.Unlock()
	return c.writeResponse(frame.ID, true, c.server.controlPlaneService().ConnectAck(connectedAt, c.userSummary()), "")
}

func (c *openClawWSConn) requireConfigRead() error {
	if HasPermission(c.user, "config.read") || HasPermission(c.user, "config.write") {
		return nil
	}
	return fmt.Errorf("forbidden: missing config.read")
}
