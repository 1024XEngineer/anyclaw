package gateway

import (
	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
)

func (s *Server) ensureChannelSession(source string, sessionID string, routed routeingress.RouteOutput, meta map[string]string, streaming bool) (string, error) {
	return s.channelBridgeService().EnsureSession(source, sessionID, routed, meta, streaming)
}
