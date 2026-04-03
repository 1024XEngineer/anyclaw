package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	appmemory "github.com/anyclaw/anyclaw/pkg/memory"
)

func MemorySearchToolWithCwd(ctx context.Context, input map[string]any, cwd string) (string, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}

	limit := 5
	if value, ok := input["limit"].(float64); ok && value > 0 {
		limit = int(value)
	}

	day, _ := input["date"].(string)
	matches, err := appmemory.SearchDailyMarkdown(dailyMemoryDir(cwd), query, limit, day)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "No daily memory matches found", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d daily memory match(es)\n\n", len(matches)))
	for _, match := range matches {
		sb.WriteString(fmt.Sprintf("[%s] %s\n", match.Date, match.Path))
		sb.WriteString(match.Snippet)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func MemoryGetToolWithCwd(ctx context.Context, input map[string]any, cwd string) (string, error) {
	day, ok := input["date"].(string)
	if !ok || strings.TrimSpace(day) == "" {
		return "", fmt.Errorf("date is required")
	}

	file, err := appmemory.GetDailyMarkdown(dailyMemoryDir(cwd), day)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("# Daily Memory %s\nPath: %s\n\n%s", file.Date, file.Path, file.Content), nil
}

func dailyMemoryDir(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = "."
	}
	return filepath.Join(cwd, "memory")
}
