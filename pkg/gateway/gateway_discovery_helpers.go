package gateway

import gatewaydiscovery "github.com/1024XEngineer/anyclaw/pkg/gateway/resources/discovery"

func (s *Server) discoveryAPI() gatewaydiscovery.API {
	if s == nil || s.discoverySvc == nil {
		return gatewaydiscovery.API{}
	}
	return gatewaydiscovery.API{Service: s.discoverySvc}
}
