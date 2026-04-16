package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/skills"
)

type skillView struct {
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

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "skills.read") &&
			!HasPermission(UserFromContext(r.Context()), "config.read") &&
			!HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "skills.read"})
			return
		}
		views, err := s.listSkillViews()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "skills.read", "skills", nil)
		writeJSON(w, http.StatusOK, views)
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		var req struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		view, err := s.setSkillEnabled(req.Name, req.Enabled)
		if err != nil {
			statusCode := http.StatusBadRequest
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				statusCode = http.StatusNotFound
			}
			writeJSON(w, statusCode, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "skills.write", view.Name, map[string]any{"enabled": view.Enabled})
		writeJSON(w, http.StatusOK, view)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) currentConfiguredSkillRefs() []config.AgentSkillRef {
	if s == nil || s.app == nil || s.app.Config == nil {
		return nil
	}
	if profile, ok := s.app.Config.ResolveMainAgentProfile(); ok {
		return append([]config.AgentSkillRef(nil), profile.Skills...)
	}
	return append([]config.AgentSkillRef(nil), s.app.Config.Agent.Skills...)
}

func (s *Server) currentEnabledSkillCount() int {
	refs := s.currentConfiguredSkillRefs()
	if len(refs) == 0 {
		if s == nil || s.app == nil {
			return 0
		}
		return len(s.app.ListSkills())
	}
	count := 0
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if !ref.Enabled {
			continue
		}
		key := normalizeSkillKey(ref.Name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		count++
	}
	return count
}

func (s *Server) loadSkillCatalog() (*skills.SkillsManager, error) {
	if s == nil || s.app == nil || s.app.Config == nil {
		return nil, fmt.Errorf("server is not initialized")
	}
	manager := skills.NewSkillsManager(s.app.Config.Skills.Dir)
	if err := manager.Load(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (s *Server) listSkillViews() ([]skillView, error) {
	manager, err := s.loadSkillCatalog()
	if err != nil {
		return nil, err
	}
	return buildSkillViews(manager, s.currentConfiguredSkillRefs()), nil
}

func buildSkillViews(manager *skills.SkillsManager, refs []config.AgentSkillRef) []skillView {
	if manager == nil {
		return nil
	}
	refIndex := make(map[string]config.AgentSkillRef, len(refs))
	for _, ref := range refs {
		key := normalizeSkillKey(ref.Name)
		if key == "" {
			continue
		}
		if _, ok := refIndex[key]; ok {
			continue
		}
		refIndex[key] = ref
	}
	defaultEnabled := len(refs) == 0
	items := make([]skillView, 0, len(manager.List()))
	for _, skill := range manager.List() {
		if skill == nil {
			continue
		}
		key := normalizeSkillKey(skill.Name)
		if key == "" {
			continue
		}
		ref, ok := refIndex[key]
		enabled := defaultEnabled
		if ok {
			enabled = ref.Enabled
		}
		items = append(items, skillView{
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

func materializeSkillRefs(installed []*skills.Skill, existing []config.AgentSkillRef) []config.AgentSkillRef {
	refIndex := make(map[string]config.AgentSkillRef, len(existing))
	for _, ref := range existing {
		key := normalizeSkillKey(ref.Name)
		if key == "" {
			continue
		}
		if _, ok := refIndex[key]; ok {
			continue
		}
		refIndex[key] = ref
	}

	items := append([]*skills.Skill(nil), installed...)
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
		key := normalizeSkillKey(skill.Name)
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

func (s *Server) setSkillEnabled(name string, enabled bool) (skillView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return skillView{}, fmt.Errorf("name is required")
	}

	manager, err := s.loadSkillCatalog()
	if err != nil {
		return skillView{}, err
	}

	refs := materializeSkillRefs(manager.List(), s.currentConfiguredSkillRefs())
	targetKey := normalizeSkillKey(name)
	found := false
	for i := range refs {
		if normalizeSkillKey(refs[i].Name) != targetKey {
			continue
		}
		refs[i].Enabled = enabled
		found = true
		break
	}
	if !found {
		return skillView{}, fmt.Errorf("skill not found: %s", name)
	}

	if err := s.applyConfiguredSkillRefs(refs); err != nil {
		return skillView{}, err
	}
	if s.runtimePool != nil {
		s.runtimePool.InvalidateAll()
	}

	for _, view := range buildSkillViews(manager, refs) {
		if normalizeSkillKey(view.Name) == targetKey {
			return view, nil
		}
	}
	return skillView{}, fmt.Errorf("skill not found: %s", name)
}

func (s *Server) applyConfiguredSkillRefs(refs []config.AgentSkillRef) error {
	if s == nil || s.app == nil || s.app.Config == nil {
		return fmt.Errorf("server is not initialized")
	}
	snapshot := append([]config.AgentSkillRef(nil), refs...)
	if profile, ok := s.app.Config.ResolveMainAgentProfile(); ok {
		profile.Skills = snapshot
		if err := s.app.Config.UpsertAgentProfile(profile); err != nil {
			return err
		}
	} else {
		s.app.Config.Agent.Skills = snapshot
	}
	return s.app.Config.Save(s.app.ConfigPath)
}

func normalizeSkillKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
