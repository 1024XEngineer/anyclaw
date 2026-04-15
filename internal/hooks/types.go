package hooks

import "anyclaw/pkg/sdk"

// HookPoint is the lifecycle stage for a hook.
type HookPoint = sdk.HookPoint

// HookContext is the shared hook execution context.
type HookContext = sdk.HookContext

// Hook is the internal alias for the public hook contract.
type Hook = sdk.Hook

// HookRegistry is the internal alias for the public hook registry contract.
type HookRegistry = sdk.HookRegistry
