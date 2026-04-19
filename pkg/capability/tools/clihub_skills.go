package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/clihub"
)

func RegisterCLIHubSkillTools(r *Registry, root string, opts BuiltinOptions) {
	cat, err := clihub.Load(root)
	if err != nil {
		return
	}

	skills := clihub.LoadSkillsForCatalog(cat)
	for name, skill := range skills {
		registerSkillTools(r, name, skill, root, opts)
	}
}

func registerSkillTools(r *Registry, harnessName string, skill *clihub.Skill, root string, opts BuiltinOptions) {
	for _, cmd := range skill.Commands {
		toolName := fmt.Sprintf("%s_%s", harnessName, cmd.Name)
		toolDesc := fmt.Sprintf("[%s] %s", cmd.Group, cmd.Description)

		r.Register(&Tool{
			Name:        toolName,
			Description: toolDesc,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type":        "array",
						"description": "Additional arguments for " + cmd.Name,
						"items":       map[string]string{"type": "string"},
					},
				},
			},
			Category:    ToolCategoryCustom,
			AccessLevel: ToolAccessPublic,
			Handler: func(ctx context.Context, input map[string]any) (string, error) {
				return auditCall(opts, toolName, input, func(ctx context.Context, input map[string]any) (string, error) {
					args, err := stringSliceFromAny(input["args"])
					if err != nil {
						return "", err
					}

					fullArgs := []string{cmd.Name}
					fullArgs = append(fullArgs, args...)

					status := clihub.EntryStatus{
						Entry: clihub.Entry{
							Name:       harnessName,
							EntryPoint: harnessName,
						},
						SourcePath: skill.SourcePath,
						DevModule:  resolveDevModuleForHarness(root, harnessName),
					}

					execOpts := CLIHubExecOptions{
						PreferLocalSrc:    true,
						RetryAfterInstall: true,
					}

					command, cwd, shellName, err := buildCLIHubCommand(status, fullArgs, true, "", opts, execOpts)
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
}

func resolveDevModuleForHarness(root string, name string) string {
	cat, err := clihub.Load(root)
	if err != nil {
		return ""
	}
	status, ok := clihub.Find(cat, name)
	if !ok {
		return ""
	}
	return status.DevModule
}

func BuildSkillSummaryForPrompt(cat *clihub.Catalog) string {
	if cat == nil {
		return ""
	}

	skills := clihub.LoadSkillsForCatalog(cat)
	if len(skills) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "## Available CLI Hub Skills")

	for name, skill := range skills {
		if len(skill.Commands) == 0 {
			continue
		}

		var cmds []string
		for _, cmd := range skill.Commands {
			cmds = append(cmds, cmd.Name)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", name, strings.Join(cmds, ", ")))
	}

	lines = append(lines, "Use these tools directly instead of calling clihub_exec manually.")

	return strings.Join(lines, "\n")
}
