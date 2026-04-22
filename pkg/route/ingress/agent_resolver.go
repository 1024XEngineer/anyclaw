package ingress

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

// AgentResolver implements M2 by combining explicit hints and the configured main agent.
type AgentResolver struct {
	Router               *Router
	ResolveMainAgentName func() string
}

// Resolve decides which agent should own this route request.
func (r AgentResolver) Resolve(request *MainRouteRequest) (AgentResolution, RouteDecision, error) {
	if request == nil {
		request = &MainRouteRequest{}
	}

	decision := RouteDecision{}
	if r.Router != nil {
		decision = r.Router.Decide(routeRequestFromMainRequest(request))
	}

	mainAgentName := r.mainAgentName()
	requestedAgentName := strings.TrimSpace(request.Hint.RequestedAgentName)

	switch {
	case requestedAgentName == "":
		if mainAgentName != "" {
			return AgentResolution{
				AgentName: mainAgentName,
				MatchedBy: "default-main",
			}, decision, nil
		}
	case config.IsMainAgentAlias(requestedAgentName):
		return AgentResolution{
			AgentName: defaultString(mainAgentName, "main"),
			MatchedBy: "requested-main",
		}, decision, nil
	case mainAgentName != "" && strings.EqualFold(requestedAgentName, mainAgentName):
		return AgentResolution{
			AgentName: mainAgentName,
			MatchedBy: "requested-main",
		}, decision, nil
	default:
		return AgentResolution{
			AgentName: requestedAgentName,
			MatchedBy: "requested",
		}, decision, nil
	}

	return AgentResolution{
		AgentName: "main",
		MatchedBy: "fallback",
	}, decision, nil
}

func (r AgentResolver) mainAgentName() string {
	if r.ResolveMainAgentName == nil {
		return ""
	}
	return strings.TrimSpace(r.ResolveMainAgentName())
}

func routeRequestFromMainRequest(request *MainRouteRequest) RouteRequest {
	if request == nil {
		return RouteRequest{}
	}

	return RouteRequest{
		Channel:   request.Scope.ChannelID,
		Source:    request.Scope.ConversationID,
		Text:      request.Text,
		ThreadID:  request.Scope.ThreadID,
		IsGroup:   request.Scope.IsGroup,
		GroupID:   request.Scope.GroupID,
		TitleHint: request.Hint.TitleHint,
	}
}
