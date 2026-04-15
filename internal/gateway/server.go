package gateway

import (
	"context"

	"anyclaw/internal/protocol"
	"anyclaw/internal/security"
)

// MethodHandler executes one gateway RPC method.
type MethodHandler interface {
	Handle(ctx context.Context, principal security.Principal, req protocol.RequestFrame) (any, *protocol.ErrorPayload)
}

// Service is the control-plane shell that will eventually host WS/HTTP transports.
type Service struct {
	handlers map[string]MethodHandler
}

// NewService creates an empty gateway service.
func NewService() *Service {
	return &Service{handlers: make(map[string]MethodHandler)}
}

// RegisterMethod installs a handler for an RPC method.
func (s *Service) RegisterMethod(method string, handler MethodHandler) {
	s.handlers[method] = handler
}

// Start starts the gateway service.
func (s *Service) Start(context.Context) error {
	return nil
}

// Shutdown gracefully stops the gateway service.
func (s *Service) Shutdown(context.Context) error {
	return nil
}
