package main

import "testing"

func TestNormalizeRootCommandSupportsOpenClawAliases(t *testing.T) {
	tests := map[string]string{
		"skill":    "skill",
		"skills":   "skill",
		"plugin":   "plugin",
		"plugins":  "plugin",
		"agent":    "agent",
		"agents":   "agent",
		"clihub":   "clihub",
		"claw":     "claw",
		"app":      "app",
		"apps":     "app",
		"channel":  "channels",
		"session":  "sessions",
		"approval": "approvals",
		"model":    "models",
		"setup":    "onboard",
		"daemon":   "daemon",
		"cron":     "cron",
		"pi":       "pi",
	}

	for input, want := range tests {
		if got := normalizeRootCommand(input); got != want {
			t.Fatalf("normalizeRootCommand(%q) = %q, want %q", input, got, want)
		}
	}
}
