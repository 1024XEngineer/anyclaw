package gateway

import (
	"fmt"
	"strings"
)

func (c *openClawWSConn) userSummary() map[string]any {
	if c.user == nil {
		return map[string]any{"name": "", "role": ""}
	}
	return map[string]any{
		"name":        c.user.Name,
		"role":        c.user.Role,
		"permissions": c.user.Permissions,
	}
}

func (c *openClawWSConn) requirePermission(permission string) error {
	if !HasPermission(c.user, permission) {
		return fmt.Errorf("forbidden: missing %s", permission)
	}
	return nil
}

func (c *openClawWSConn) writeResponse(id string, ok bool, data any, errMsg string) error {
	frame := openClawWSFrame{
		Type: "res",
		ID:   id,
		OK:   ok,
		Data: data,
	}
	if strings.TrimSpace(errMsg) != "" {
		frame.Error = strings.TrimSpace(errMsg)
	}
	return c.writeFrame(frame)
}

func (c *openClawWSConn) writeError(id string, errMsg string) error {
	return c.writeResponse(id, false, nil, errMsg)
}

func (c *openClawWSConn) writeFrame(frame openClawWSFrame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(frame)
}

func (c *openClawWSConn) shutdown() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.stopEventStream()
		_ = c.conn.Close()
	})
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func mapInt(values map[string]any, key string, fallback int) int {
	if values == nil {
		return fallback
	}
	switch value := values[key].(type) {
	case float64:
		if int(value) > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	}
	return fallback
}
