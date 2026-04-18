package gateway

import (
	"context"
	"fmt"
	"os"

	"github.com/1024XEngineer/anyclaw/pkg/gateway/resources/discovery"
	nodepkg "github.com/1024XEngineer/anyclaw/pkg/gateway/resources/nodes"
)

func (s *Server) initDiscovery(ctx context.Context) {
	port := s.mainRuntime.Config.Gateway.Port
	if port <= 0 {
		port = 18789
	}

	caps := []string{"gateway", "chat", "agents"}
	for _, profile := range s.mainRuntime.Config.Agent.Profiles {
		caps = append(caps, "agent:"+profile.Name)
	}

	svc := discovery.NewService(discovery.Config{
		ServiceName:  s.mainRuntime.Config.Agent.Name,
		ServicePort:  port,
		InstanceID:   fmt.Sprintf("anyclaw-%s", s.mainRuntime.Config.Agent.Name),
		Version:      "1.0.0",
		Capabilities: caps,
		Metadata: map[string]string{
			"provider": s.mainRuntime.Config.LLM.Provider,
			"model":    s.mainRuntime.Config.LLM.Model,
		},
	})

	svc.OnDiscover(func(inst *discovery.Instance) {
		s.appendEvent("discovery.instance", "", map[string]any{
			"event":   "discovered",
			"id":      inst.ID,
			"name":    inst.Name,
			"url":     inst.URL,
			"version": inst.Version,
			"caps":    inst.Caps,
		})

		if inst.Address != "" && !discovery.IsLocalhost(inst.Address) {
			node := &nodepkg.Device{
				ID:           inst.ID,
				Name:         inst.Name,
				Type:         "anyclaw",
				Capabilities: inst.Caps,
				Status:       "online",
				ConnectedAt:  inst.LastSeen,
				LastSeen:     inst.LastSeen,
				Metadata: map[string]string{
					"url":     inst.URL,
					"version": inst.Version,
				},
			}
			s.nodes.Register(node)
		}
	})

	if err := svc.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "discovery start error: %v\n", err)
		return
	}

	s.discoverySvc = svc

	go func() {
		<-ctx.Done()
		svc.Stop()
	}()
}
