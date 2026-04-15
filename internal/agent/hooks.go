package agent

import (
	"context"

	"anyclaw/internal/hooks"
	"anyclaw/pkg/sdk"
)

// RunHooks executes all hooks for one hook point.
func RunHooks(ctx context.Context, registry *hooks.Registry, point sdk.HookPoint, hookContext sdk.HookContext) error {
	for _, hook := range registry.List(point) {
		if err := hook.Run(ctx, hookContext); err != nil {
			return err
		}
	}
	return nil
}
