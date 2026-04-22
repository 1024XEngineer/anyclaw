package controlplane

import "time"

// ConnectAck is the stable control-plane handshake payload returned to WS clients.
type ConnectAck struct {
	Status      string         `json:"status"`
	Protocol    string         `json:"protocol"`
	ConnectedAt string         `json:"connected_at"`
	User        map[string]any `json:"user"`
	Methods     []string       `json:"methods"`
}

// MethodsCatalog is the response payload used by methods.list.
type MethodsCatalog struct {
	Methods []string `json:"methods"`
}

// PingResult is the stable control-plane liveness response.
type PingResult struct {
	Pong string `json:"pong"`
}

// SubscriptionResult is the stable response for event stream subscription toggles.
type SubscriptionResult struct {
	Subscribed bool `json:"subscribed"`
}

// Service builds stable control-plane payloads for gateway clients.
type Service struct {
	protocol string
	methods  []string
}

// NewService creates a control-plane payload builder.
func NewService(protocol string, methods []string) Service {
	cloned := append([]string(nil), methods...)
	return Service{protocol: protocol, methods: cloned}
}

// ConnectAck returns the normalized hello/connect response for WS clients.
func (s Service) ConnectAck(connectedAt time.Time, user map[string]any) ConnectAck {
	return ConnectAck{
		Status:      "connected",
		Protocol:    s.protocol,
		ConnectedAt: connectedAt.UTC().Format(time.RFC3339),
		User:        user,
		Methods:     append([]string(nil), s.methods...),
	}
}

// MethodsList returns the supported control-plane methods.
func (s Service) MethodsList() MethodsCatalog {
	return MethodsCatalog{Methods: append([]string(nil), s.methods...)}
}

// Ping returns the normalized liveness response for WS clients.
func (s Service) Ping(now time.Time) PingResult {
	return PingResult{Pong: now.UTC().Format(time.RFC3339)}
}

// Subscription returns the normalized event subscription response.
func (s Service) Subscription(subscribed bool) SubscriptionResult {
	return SubscriptionResult{Subscribed: subscribed}
}
