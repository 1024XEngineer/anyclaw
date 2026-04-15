package security

import "anyclaw/internal/protocol"

// Principal is the authenticated identity that executes gateway requests.
type Principal struct {
	Role     Role
	DeviceID string
	Scopes   []string
	Client   protocol.ClientInfo
}
