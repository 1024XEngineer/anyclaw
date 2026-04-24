package gateway

import (
	"context"
	"fmt"
	"strings"
)

func (c *openClawWSConn) handleRequest(ctx context.Context, frame openClawWSFrame) error {
	method := strings.TrimSpace(frame.Method)
	if method == "" {
		return fmt.Errorf("method is required")
	}
	if err := c.ensureConnected(method); err != nil {
		return err
	}
	normalized := strings.ToLower(method)
	handlers := []func(context.Context, openClawWSFrame, string) (bool, error){
		c.handleCoreWSRequest,
		c.handleCatalogWSRequest,
		c.handleMutationWSRequest,
		c.handleDeviceWSRequest,
	}
	for _, handler := range handlers {
		handled, err := handler(ctx, frame, normalized)
		if handled {
			return err
		}
	}
	return fmt.Errorf("unsupported method: %s", method)
}

func (c *openClawWSConn) ensureConnected(method string) error {
	c.connMu.RLock()
	connected := c.connected
	c.connMu.RUnlock()
	if !connected && !strings.EqualFold(method, "connect") {
		return fmt.Errorf("connect required before calling %s", method)
	}
	return nil
}
