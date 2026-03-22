package agent

import (
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func BuildPersonalityPrompt(spec config.PersonalitySpec) string {
	parts := make([]string, 0, 8)
	if v := strings.TrimSpace(spec.Template); v != "" {
		parts = append(parts, "Personality template: "+v)
	}
	if v := strings.TrimSpace(spec.Tone); v != "" {
		parts = append(parts, "Tone: "+v)
	}
	if v := strings.TrimSpace(spec.Style); v != "" {
		parts = append(parts, "Style: "+v)
	}
	if v := strings.TrimSpace(spec.GoalOrientation); v != "" {
		parts = append(parts, "Goal orientation: "+v)
	}
	if v := strings.TrimSpace(spec.ConstraintMode); v != "" {
		parts = append(parts, "Constraint mode: "+v)
	}
	if v := strings.TrimSpace(spec.ResponseVerbosity); v != "" {
		parts = append(parts, "Response verbosity: "+v)
	}
	if len(spec.Traits) > 0 {
		parts = append(parts, "Traits: "+strings.Join(spec.Traits, ", "))
	}
	if v := strings.TrimSpace(spec.CustomInstructions); v != "" {
		parts = append(parts, "Custom instructions: "+v)
	}
	return strings.Join(parts, "\n")
}
