package gateway

import (
	"context"

	nodepkg "github.com/1024XEngineer/anyclaw/pkg/gateway/resources/nodes"
)

func (s *Server) nodesAPI() nodepkg.API {
	return nodepkg.API{
		Nodes: s.nodes,
		AppendAudit: func(ctx context.Context, action string, target string, meta map[string]any) {
			s.appendAudit(UserFromContext(ctx), action, target, meta)
		},
		WriteJSON: writeJSON,
	}
}
