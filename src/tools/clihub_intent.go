package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/clihub"
)

type IntentRouterTool struct {
	router *clihub.IntentRouter
	opts   BuiltinOptions
}

func RegisterIntentRouterTool(r *Registry, root string, opts BuiltinOptions) {
	router, err := clihub.NewIntentRouter(root)
	if err != nil {
		return
	}

	tool := &IntentRouterTool{
		router: router,
		opts:   opts,
	}

	r.Register(&Tool{
		Name:        "intent_route",
		Description: "Route a natural language request to the appropriate CLI Hub capability and execute it automatically.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"intent": map[string]string{"type": "string", "description": "Natural language intent (e.g., 'create a new video project', 'list all models')"},
				"args": map[string]any{
					"type":        "array",
					"description": "Additional command arguments",
					"items":       map[string]string{"type": "string"},
				},
				"json": map[string]string{"type": "boolean", "description": "Request JSON output (default true)"},
			},
			"required": []string{"intent"},
		},
		Category:    ToolCategoryCustom,
		AccessLevel: ToolAccessPublic,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "intent_route", input, func(ctx context.Context, input map[string]any) (string, error) {
				intentStr, _ := input["intent"].(string)
				if intentStr == "" {
					return "", fmt.Errorf("intent is required")
				}

				args, _ := input["args"].([]string)
				if args == nil {
					if raw, ok := input["args"].([]any); ok {
						args = make([]string, len(raw))
						for i, v := range raw {
							args[i] = fmt.Sprint(v)
						}
					}
				}

				jsonMode := true
				if raw, exists := input["json"]; exists {
					if b, ok := raw.(bool); ok {
						jsonMode = b
					}
				}

				cap := tool.router.Registry.BestMatch(intentStr)
				if cap == nil {
					matches := tool.router.Registry.FindByIntent(intentStr)
					if len(matches) == 0 {
						return "", fmt.Errorf("no matching capability found for: %s", intentStr)
					}
					cap = &matches[0]
				}

				if jsonMode {
					args = append([]string{"--json"}, args...)
				}

				cmdArgs, cwd, err := clihub.ResolveCapabilityPath("", *cap)
				if err != nil {
					return "", err
				}

				fullArgs := append(cmdArgs, cap.Command)
				fullArgs = append(fullArgs, args...)

				shellName := defaultCLIHubShell()
				command := joinArgsForShell(fullArgs, shellName)

				return RunCommandToolWithPolicy(ctx, map[string]any{
					"command": command,
					"cwd":     cwd,
					"shell":   shellName,
				}, opts)
			})(ctx, input)
		},
	})

	r.Register(&Tool{
		Name:        "intent_list_capabilities",
		Description: "List all available CLI Hub capabilities that can be auto-routed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":   map[string]string{"type": "string", "description": "Optional search query to filter capabilities"},
				"harness": map[string]string{"type": "string", "description": "Optional harness name to filter by"},
				"limit":   map[string]string{"type": "number", "description": "Maximum number of results (default 20)"},
			},
		},
		Category:    ToolCategoryCustom,
		AccessLevel: ToolAccessPublic,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "intent_list_capabilities", input, func(ctx context.Context, input map[string]any) (string, error) {
				query, _ := input["query"].(string)
				harness, _ := input["harness"].(string)
				limit := intFromAny(input["limit"], 20)

				var caps []clihub.Capability
				if query != "" {
					caps = tool.router.Registry.FindByIntent(query)
				} else if harness != "" {
					caps = tool.router.Registry.FindByHarness(harness)
				} else {
					caps = tool.router.Registry.All()
				}

				if limit > 0 && len(caps) > limit {
					caps = caps[:limit]
				}

				return mustMarshalJSON(map[string]any{
					"count":        len(caps),
					"capabilities": caps,
				})
			})(ctx, input)
		},
	})
}

func RegisterIntentRouterToolIfNeeded(r *Registry, root string, opts BuiltinOptions) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	RegisterIntentRouterTool(r, root, opts)
}
