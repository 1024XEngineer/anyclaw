package ingress

import "testing"

func TestAgentResolverUsesRequestedAndMainAgentFallback(t *testing.T) {
	resolver := AgentResolver{
		ResolveMainAgentName: func() string {
			return "AnyClaw"
		},
	}

	defaultResolution, decision, err := resolver.Resolve(&MainRouteRequest{})
	if err != nil {
		t.Fatalf("Resolve default: %v", err)
	}
	if decision.RouteKey != "" {
		t.Fatalf("expected empty decision without router, got %#v", decision)
	}
	if defaultResolution.AgentName != "AnyClaw" || defaultResolution.MatchedBy != "default-main" {
		t.Fatalf("expected default main agent resolution, got %#v", defaultResolution)
	}

	mainAliasResolution, _, err := resolver.Resolve(&MainRouteRequest{
		Hint: RouteHint{RequestedAgentName: "main"},
	})
	if err != nil {
		t.Fatalf("Resolve main alias: %v", err)
	}
	if mainAliasResolution.AgentName != "AnyClaw" || mainAliasResolution.MatchedBy != "requested-main" {
		t.Fatalf("expected requested main agent resolution, got %#v", mainAliasResolution)
	}

	specialistResolution, _, err := resolver.Resolve(&MainRouteRequest{
		Hint: RouteHint{RequestedAgentName: "vision-agent"},
	})
	if err != nil {
		t.Fatalf("Resolve specialist: %v", err)
	}
	if specialistResolution.AgentName != "vision-agent" || specialistResolution.MatchedBy != "requested" {
		t.Fatalf("expected requested specialist resolution, got %#v", specialistResolution)
	}
}
