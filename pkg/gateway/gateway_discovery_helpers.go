package gateway

import gatewaydiscovery "github.com/anyclaw/anyclaw/pkg/gateway/resources/discovery"

func (s *Server) discoveryAPI() gatewaydiscovery.API {
	return gatewaydiscovery.API{Service: s.discoverySvc}
}
