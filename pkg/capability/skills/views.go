package skills

import (
	"sort"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

// View is the control-plane projection of an installed skill.
type View struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Entrypoint  string   `json:"entrypoint,omitempty"`
	Registry    string   `json:"registry,omitempty"`
	Source      string   `json:"source,omitempty"`
	InstallHint string   `json:"installHint,omitempty"`
	Enabled     bool     `json:"enabled"`
	Loaded      bool     `json:"loaded"`
}

// BuildViews projects installed skills with their configured enablement state.
func BuildViews(manager *SkillsManager, refs []config.AgentSkillRef) []View {
	if manager == nil {
		return nil
	}

	refIndex := make(map[string]config.AgentSkillRef, len(refs))
	for _, ref := range refs {
		key := NormalizeKey(ref.Name)
		if key == "" {
			continue
		}
		if _, ok := refIndex[key]; ok {
			continue
		}
		refIndex[key] = ref
	}

	defaultEnabled := len(refs) == 0
	items := make([]View, 0, len(manager.List()))
	for _, skill := range manager.List() {
		if skill == nil {
			continue
		}
		key := NormalizeKey(skill.Name)
		if key == "" {
			continue
		}
		ref, ok := refIndex[key]
		enabled := defaultEnabled
		if ok {
			enabled = ref.Enabled
		}
		items = append(items, View{
			Name:        skill.Name,
			Description: skill.Description,
			Version:     skill.Version,
			Permissions: append([]string(nil), skill.Permissions...),
			Entrypoint:  skill.Entrypoint,
			Registry:    skill.Registry,
			Source:      skill.Source,
			InstallHint: skill.InstallCommand,
			Enabled:     enabled,
			Loaded:      enabled,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Loaded != items[j].Loaded {
			return items[i].Loaded && !items[j].Loaded
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items
}

// MaterializeRefs expands configured refs so every installed skill gets a stable config row.
func MaterializeRefs(installed []*Skill, existing []config.AgentSkillRef) []config.AgentSkillRef {
	refIndex := make(map[string]config.AgentSkillRef, len(existing))
	for _, ref := range existing {
		key := NormalizeKey(ref.Name)
		if key == "" {
			continue
		}
		if _, ok := refIndex[key]; ok {
			continue
		}
		refIndex[key] = ref
	}

	items := append([]*Skill(nil), installed...)
	sort.Slice(items, func(i, j int) bool {
		left := ""
		if items[i] != nil {
			left = strings.ToLower(strings.TrimSpace(items[i].Name))
		}
		right := ""
		if items[j] != nil {
			right = strings.ToLower(strings.TrimSpace(items[j].Name))
		}
		return left < right
	})

	defaultEnabled := len(existing) == 0
	refs := make([]config.AgentSkillRef, 0, len(items)+len(existing))
	seen := make(map[string]struct{}, len(items))
	for _, skill := range items {
		if skill == nil {
			continue
		}
		key := NormalizeKey(skill.Name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		ref, ok := refIndex[key]
		if ok {
			ref.Name = skill.Name
			if strings.TrimSpace(ref.Version) == "" {
				ref.Version = skill.Version
			}
			if len(ref.Permissions) == 0 && len(skill.Permissions) > 0 {
				ref.Permissions = append([]string(nil), skill.Permissions...)
			}
		} else {
			ref = config.AgentSkillRef{
				Name:        skill.Name,
				Enabled:     defaultEnabled,
				Permissions: append([]string(nil), skill.Permissions...),
				Version:     skill.Version,
			}
		}
		refs = append(refs, ref)
	}

	missingKeys := make([]string, 0)
	for key := range refIndex {
		if _, ok := seen[key]; ok {
			continue
		}
		missingKeys = append(missingKeys, key)
	}
	sort.Strings(missingKeys)
	for _, key := range missingKeys {
		refs = append(refs, refIndex[key])
	}
	return refs
}

// NormalizeKey gives skills a stable case-insensitive identity.
func NormalizeKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
