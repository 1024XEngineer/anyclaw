package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
)

func BridgeToToolRegistry(registry *tools.Registry, mcpRegistry *Registry) error {
	allTools := mcpRegistry.AllTools()

	for serverName, mcpTools := range allTools {
		for _, mcpTool := range mcpTools {
			toolName := fmt.Sprintf("mcp__%s__%s", serverName, mcpTool.Name)

			schema := mcpTool.InputSchema
			if schema == nil {
				schema = map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				}
			}

			serverName := serverName
			mcpToolName := mcpTool.Name

			registry.RegisterTool(toolName, mcpTool.Description, schema,
				func(ctx context.Context, input map[string]any) (string, error) {
					result, err := mcpRegistry.CallTool(ctx, serverName, mcpToolName, input)
					if err != nil {
						return "", fmt.Errorf("MCP %s/%s: %w", serverName, mcpToolName, err)
					}
					return formatMCPResult(result)
				})
		}
	}

	return nil
}

func formatMCPResult(result any) (string, error) {
	if result == nil {
		return "", nil
	}

	if resultObj, ok := result.(map[string]any); ok {
		if contentRaw, ok := resultObj["content"]; ok {
			if contentArr, ok := contentRaw.([]any); ok {
				var textParts []string
				for _, item := range contentArr {
					if itemMap, ok := item.(map[string]any); ok {
						if text, ok := itemMap["text"].(string); ok {
							textParts = append(textParts, text)
						}
					}
				}
				if len(textParts) > 0 {
					return joinStrings(textParts, "\n\n"), nil
				}
			}
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", result), nil
	}
	return string(data), nil
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
