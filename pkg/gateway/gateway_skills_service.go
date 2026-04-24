package gateway

import (
	"fmt"
	"strings"

	skillscatalog "github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func (s *Server) currentConfiguredSkillRefs() []config.AgentSkillRef {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return nil
	}
	if profile, ok := s.mainRuntime.Config.ResolveMainAgentProfile(); ok {
		return append([]config.AgentSkillRef(nil), profile.Skills...)
	}
	return append([]config.AgentSkillRef(nil), s.mainRuntime.Config.Agent.Skills...)
}

func (s *Server) currentEnabledSkillCount() int {
	refs := s.currentConfiguredSkillRefs()
	if len(refs) == 0 {
		if s == nil || s.mainRuntime == nil {
			return 0
		}
		return len(s.mainRuntime.ListSkills())
	}
	count := 0
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if !ref.Enabled {
			continue
		}
		key := skillscatalog.NormalizeKey(ref.Name)
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

func (s *Server) loadSkillCatalog() (*skillscatalog.SkillsManager, error) {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return nil, fmt.Errorf("server is not initialized")
	}
	manager := skillscatalog.NewSkillsManager(s.mainRuntime.Config.Skills.Dir)
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
	return skillscatalog.BuildViews(manager, s.currentConfiguredSkillRefs()), nil
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

	refs := skillscatalog.MaterializeRefs(manager.List(), s.currentConfiguredSkillRefs())
	targetKey := skillscatalog.NormalizeKey(name)
	found := false
	for i := range refs {
		if skillscatalog.NormalizeKey(refs[i].Name) != targetKey {
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

	for _, view := range skillscatalog.BuildViews(manager, refs) {
		if skillscatalog.NormalizeKey(view.Name) == targetKey {
			return view, nil
		}
	}
	return skillView{}, fmt.Errorf("skill not found: %s", name)
}

func (s *Server) applyConfiguredSkillRefs(refs []config.AgentSkillRef) error {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return fmt.Errorf("server is not initialized")
	}
	snapshot := append([]config.AgentSkillRef(nil), refs...)
	if profile, ok := s.mainRuntime.Config.ResolveMainAgentProfile(); ok {
		profile.Skills = snapshot
		if err := s.mainRuntime.Config.UpsertAgentProfile(profile); err != nil {
			return err
		}
	} else {
		s.mainRuntime.Config.Agent.Skills = snapshot
	}
	return s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath)
}
