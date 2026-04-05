package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/clihub"
)

type CLIHubExecOptions struct {
	AutoInstall       bool
	PreferLocalSrc    bool
	RetryAfterInstall bool
}

func RegisterCLIHubTools(r *Registry, opts BuiltinOptions) {
	root, ok := clihub.DiscoverRoot(opts.WorkingDir)
	if !ok {
		return
	}

	RegisterCLIHubSkillTools(r, root, opts)
	RegisterIntentRouterTool(r, root, opts)

	r.Register(&Tool{
		Name:        "clihub_catalog",
		Description: "Search the local CLI-Anything catalog, inspect install hints, and see which harnesses are already available to AnyClaw.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":          map[string]string{"type": "string", "description": "Search query"},
				"category":       map[string]string{"type": "string", "description": "Optional category filter"},
				"installed_only": map[string]string{"type": "boolean", "description": "Return only installed harnesses"},
				"limit":          map[string]string{"type": "number", "description": "Maximum number of results"},
			},
		},
		Category:    ToolCategoryCustom,
		AccessLevel: ToolAccessPublic,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "clihub_catalog", input, func(ctx context.Context, input map[string]any) (string, error) {
				cat, err := clihub.Load(root)
				if err != nil {
					return "", err
				}
				query, _ := input["query"].(string)
				category, _ := input["category"].(string)
				installedOnly, _ := input["installed_only"].(bool)
				limit := intFromAny(input["limit"], 8)
				results := clihub.Search(cat, query, category, installedOnly, limit)
				return mustMarshalJSON(map[string]any{
					"root":           cat.Root,
					"updated":        cat.Updated,
					"query":          strings.TrimSpace(query),
					"category":       strings.TrimSpace(category),
					"installed_only": installedOnly,
					"count":          len(results),
					"results":        results,
				})
			})(ctx, input)
		},
	})

	r.Register(&Tool{
		Name:        "clihub_exec",
		Description: "Execute a discovered CLI-Anything harness with JSON-first defaults and safer fallback behavior than raw shell usage. Supports auto-install and local source fallback.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]string{"type": "string", "description": "Catalog name, display name, or entry point"},
				"args": map[string]any{
					"type":        "array",
					"description": "CLI arguments after the executable name",
					"items":       map[string]string{"type": "string"},
				},
				"cwd":          map[string]string{"type": "string", "description": "Optional working directory override for installed commands"},
				"json":         map[string]string{"type": "boolean", "description": "Inject --json when appropriate (default true)"},
				"auto_install": map[string]string{"type": "boolean", "description": "Auto-install if not installed and install_cmd available (default false)"},
			},
			"required": []string{"name", "args"},
		},
		Category:    ToolCategoryCustom,
		AccessLevel: ToolAccessPublic,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "clihub_exec", input, func(ctx context.Context, input map[string]any) (string, error) {
				cat, err := clihub.Load(root)
				if err != nil {
					return "", err
				}
				name, _ := input["name"].(string)
				status, ok := clihub.Find(cat, name)
				if !ok {
					return "", fmt.Errorf("CLI Hub entry not found: %s", name)
				}

				args, err := stringSliceFromAny(input["args"])
				if err != nil {
					return "", err
				}
				if len(args) == 0 {
					return "", fmt.Errorf("args are required; CLI-Anything defaults to interactive REPL when no command is provided")
				}

				jsonMode := true
				if raw, exists := input["json"]; exists {
					if value, ok := raw.(bool); ok {
						jsonMode = value
					}
				}

				autoInstall := false
				if raw, exists := input["auto_install"]; exists {
					if value, ok := raw.(bool); ok {
						autoInstall = value
					}
				}

				execOpts := CLIHubExecOptions{
					AutoInstall:       autoInstall,
					PreferLocalSrc:    true,
					RetryAfterInstall: true,
				}

				command, cwd, shellName, err := buildCLIHubCommand(status, args, jsonMode, stringValueCLIHub(input["cwd"]), opts, execOpts)
				if err != nil {
					return "", err
				}
				return RunCommandToolWithPolicy(ctx, map[string]any{
					"command": command,
					"cwd":     cwd,
					"shell":   shellName,
				}, opts)
			})(ctx, input)
		},
	})
}

func buildCLIHubCommand(status clihub.EntryStatus, args []string, jsonMode bool, requestedCwd string, opts BuiltinOptions, execOpts CLIHubExecOptions) (string, string, string, error) {
	resolved, err := clihub.ResolveCommand(status, args, clihub.ExecOptions{
		JSON:              jsonMode,
		AutoInstall:       execOpts.AutoInstall,
		PreferLocalSrc:    execOpts.PreferLocalSrc,
		RetryAfterInstall: execOpts.RetryAfterInstall,
		RequestedCwd:      requestedCwd,
	})
	if err != nil {
		return "", "", "", err
	}

	command := resolved.ShellCommand()
	if err := reviewCommandExecution(command, resolved.Cwd, opts); err != nil {
		return "", "", "", err
	}
	return command, resolved.Cwd, resolved.Shell, nil
}

func resolveCLIHubInvocation(status clihub.EntryStatus, requestedCwd string, execOpts CLIHubExecOptions) ([]string, string, error) {
	return clihub.ResolveInvocation(status, requestedCwd, clihub.ExecOptions{
		AutoInstall:       execOpts.AutoInstall,
		PreferLocalSrc:    execOpts.PreferLocalSrc,
		RetryAfterInstall: execOpts.RetryAfterInstall,
	})
}

func runCLIHubInstall(installCmd string, name string) error {
	return clihub.RunInstall(clihub.EntryStatus{
		Entry: clihub.Entry{
			Name:       name,
			InstallCmd: installCmd,
		},
	})
}

func shouldInjectCLIHubJSON(args []string) bool {
	return clihub.ShouldInjectJSON(args)
}

func defaultCLIHubShell() string {
	return clihub.DefaultShell()
}

func joinArgsForShell(args []string, shellName string) string {
	return clihub.JoinArgsForShell(args, shellName)
}

func quoteArgForShell(value string, shellName string) string {
	return clihub.QuoteArgForShell(value, shellName)
}

func stringSliceFromAny(value any) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("args must be an array of strings")
	}
}

func stringValueCLIHub(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func mustMarshalJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
