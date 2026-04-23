package governance

import (
	"context"
	"fmt"
	"strings"
)

// AuthorizeConnection performs governance checks for one control-plane connection.
func (s Service) AuthorizeConnection(ctx context.Context, req ConnectionRequest) (ConnectionAuthorization, error) {
	actor := actorFromContext(s.currentUser(ctx), RawRequest{
		SourceType: "control",
		EntryPoint: firstNonEmpty(req.Protocol, "ws"),
		Metadata:   req.Metadata,
	})
	if !actor.Authenticated {
		return ConnectionAuthorization{}, fmt.Errorf("unauthorized")
	}
	return ConnectionAuthorization{
		Caller: actor,
		Result: GovernanceResult{
			Authenticated: true,
			PermissionSet: cloneStrings(actor.Roles),
			RateLimitKey:  buildRateLimitKey("control", strings.TrimSpace(req.Protocol), strings.TrimSpace(req.ClientID), actor.UserID),
			RiskLevel:     "normal",
		},
	}, nil
}

// AuthorizeCommand performs governance checks for one control-plane command.
func (s Service) AuthorizeCommand(ctx context.Context, req CommandRequest) (CommandAuthorization, error) {
	actor := actorFromContext(s.currentUser(ctx), RawRequest{
		SourceType: "control",
		EntryPoint: "command",
	})
	permission := strings.TrimSpace(req.RequiredPermission)
	if permission != "" && !s.hasPermission(ctx, permission) {
		return CommandAuthorization{}, fmt.Errorf("forbidden: missing %s", permission)
	}
	return CommandAuthorization{
		Request: req,
		Caller:  actor,
		Result: GovernanceResult{
			Authenticated: actor.Authenticated,
			PermissionSet: cloneStrings(actor.Roles),
			RateLimitKey:  buildRateLimitKey("command", strings.TrimSpace(req.Method), strings.TrimSpace(req.ResourceID), actor.UserID),
			RiskLevel:     "normal",
		},
	}, nil
}
