package gateway

import (
	"context"
	"strings"
	"sync"
	"time"
)

const openClawWSReadTimeout = 90 * time.Second

func (c *openClawWSConn) run(ctx context.Context) {
	defer c.shutdown()
	_ = c.conn.SetReadDeadline(time.Now().Add(openClawWSReadTimeout))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(openClawWSReadTimeout))
	})
	if err := c.writeFrame(openClawWSFrame{
		Type:  "event",
		Event: "connect.challenge",
		Data: map[string]any{
			"nonce":    c.challenge,
			"protocol": "openclaw.gateway.v1",
			"methods":  openClawWSMethods,
		},
	}); err != nil {
		return
	}
	var handlerWg sync.WaitGroup
	defer func() {
		handlerWg.Wait()
	}()
	for {
		var frame openClawWSFrame
		if err := c.conn.ReadJSON(&frame); err != nil {
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(openClawWSReadTimeout))
		if !strings.EqualFold(strings.TrimSpace(frame.Type), "req") {
			_ = c.writeError(frame.ID, "frame type must be req")
			continue
		}
		handlerWg.Add(1)
		go func(f openClawWSFrame) {
			defer handlerWg.Done()
			if err := c.handleRequest(ctx, f); err != nil {
				_ = c.writeError(f.ID, err.Error())
			}
		}(frame)
	}
}
