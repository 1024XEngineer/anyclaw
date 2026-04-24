package intake

import "strings"

func NormalizeSingleAgentSessionMode(mode string, fallback string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return fallback
	}
	if IsGroupSessionMode(mode) {
		return fallback
	}
	return mode
}

func IsGroupSessionMode(mode string) bool {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "group", "group-shared", "channel-group":
		return true
	default:
		return false
	}
}
