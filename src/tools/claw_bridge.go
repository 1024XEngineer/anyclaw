package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anyclaw/anyclaw/pkg/clawbridge"
)

func RegisterClawBridgeTools(r *Registry, opts BuiltinOptions) {
	root, ok := clawbridge.DiscoverRoot(opts.WorkingDir)
	if !ok {
		return
	}

	r.Register(&Tool{
		Name:        "claw_bridge_context",
		Description: "Read the integrated claw-code-main capability surface and orchestration hints available in this workspace.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section": map[string]string{"type": "string", "description": "summary, commands, tools, or subsystems"},
				"family":  map[string]string{"type": "string", "description": "Optional command/tool family or subsystem name"},
				"limit":   map[string]string{"type": "number", "description": "Maximum items to return"},
			},
		},
		Category:    ToolCategoryCustom,
		AccessLevel: ToolAccessPublic,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "claw_bridge_context", input, func(ctx context.Context, input map[string]any) (string, error) {
				summary, err := clawbridge.Load(root)
				if err != nil {
					return "", err
				}
				section, _ := input["section"].(string)
				family, _ := input["family"].(string)
				limit := intFromAny(input["limit"], 6)
				return clawbridge.RenderJSON(summary, section, family, limit)
			})(ctx, input)
		},
	})
}

func intFromAny(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		var n json.Number = json.Number(v)
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
	}
	return fallback
}

func ClawBridgeStatus(root string) (string, error) {
	summary, err := clawbridge.Load(root)
	if err != nil {
		return "", err
	}
	return clawbridge.HumanSummary(summary), nil
}

func ClawBridgeLookup(root string, section string, family string, limit int) (string, error) {
	summary, err := clawbridge.Load(root)
	if err != nil {
		return "", err
	}
	output, err := clawbridge.RenderJSON(summary, section, family, limit)
	if err != nil {
		return "", err
	}
	return output, nil
}

func RequireClawBridgeRoot(start string) (string, error) {
	root, ok := clawbridge.DiscoverRoot(start)
	if !ok {
		return "", fmt.Errorf("claw-code-main reference not found; set %s or run from a workspace that contains a sibling claw-code-main directory", clawbridge.EnvRoot)
	}
	return root, nil
}
