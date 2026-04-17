package gateway

import gatewayintake "github.com/anyclaw/anyclaw/pkg/gateway/intake"

func (s *Server) mainEntryPolicy() gatewayintake.MainEntryPolicy {
	return gatewayintake.MainEntryPolicy{
		ResolveMainAgentName: func() string {
			if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
				return ""
			}
			return s.mainRuntime.Config.ResolveMainAgentName()
		},
	}
}
