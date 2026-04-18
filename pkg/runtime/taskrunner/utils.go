package taskrunner

import (
	"encoding/json"
	"fmt"
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for k, v := range input {
		result[k] = v
	}
	return result
}

func approvalSignature(toolName string, action string, payload map[string]any) string {
	encoded, _ := json.Marshal(payload)
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(toolName), strings.TrimSpace(action), string(encoded))
}
