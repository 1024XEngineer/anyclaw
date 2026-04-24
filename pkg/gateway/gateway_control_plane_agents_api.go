package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") && !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agents.read", "agents", nil)
		writeJSON(w, http.StatusOK, s.listAgentViews())
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		var req struct {
			Name            string                 `json:"name"`
			Description     string                 `json:"description"`
			Role            string                 `json:"role"`
			Persona         string                 `json:"persona"`
			AvatarPreset    *string                `json:"avatar_preset"`
			AvatarDataURL   *string                `json:"avatar_data_url"`
			WorkingDir      string                 `json:"working_dir"`
			PermissionLevel string                 `json:"permission_level"`
			ProviderRef     string                 `json:"provider_ref"`
			DefaultModel    string                 `json:"default_model"`
			Enabled         *bool                  `json:"enabled"`
			Personality     config.PersonalitySpec `json:"personality"`
			Skills          []config.AgentSkillRef `json:"skills"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		profile := config.AgentProfile{
			Name:            req.Name,
			Description:     req.Description,
			Role:            req.Role,
			Persona:         req.Persona,
			WorkingDir:      req.WorkingDir,
			PermissionLevel: req.PermissionLevel,
			ProviderRef:     req.ProviderRef,
			DefaultModel:    req.DefaultModel,
			Enabled:         req.Enabled,
			Personality:     req.Personality,
			Skills:          req.Skills,
		}
		if req.AvatarPreset != nil {
			profile.AvatarPreset = *req.AvatarPreset
		}
		if req.AvatarDataURL != nil {
			profile.AvatarDataURL = *req.AvatarDataURL
		}
		if existing, ok := s.mainRuntime.Config.FindAgentProfile(profile.Name); ok {
			if req.AvatarPreset == nil {
				profile.AvatarPreset = existing.AvatarPreset
			}
			if req.AvatarDataURL == nil {
				profile.AvatarDataURL = existing.AvatarDataURL
			}
			profile.Domain = existing.Domain
			profile.Expertise = append([]string{}, existing.Expertise...)
			profile.SystemPrompt = existing.SystemPrompt
			if strings.TrimSpace(profile.Personality.Template) == "" &&
				strings.TrimSpace(profile.Personality.Tone) == "" &&
				strings.TrimSpace(profile.Personality.Style) == "" &&
				strings.TrimSpace(profile.Personality.GoalOrientation) == "" &&
				strings.TrimSpace(profile.Personality.ConstraintMode) == "" &&
				strings.TrimSpace(profile.Personality.ResponseVerbosity) == "" &&
				strings.TrimSpace(profile.Personality.CustomInstructions) == "" &&
				len(profile.Personality.Traits) == 0 {
				profile.Personality = existing.Personality
			}
		}
		if profile.Enabled == nil {
			profile.Enabled = config.BoolPtr(true)
		}
		if strings.TrimSpace(profile.PermissionLevel) == "" {
			profile.PermissionLevel = "limited"
		}
		if strings.TrimSpace(profile.ProviderRef) != "" {
			if _, ok := s.mainRuntime.Config.FindProviderProfile(profile.ProviderRef); !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider not found"})
				return
			}
		}
		if err := s.mainRuntime.Config.UpsertAgentProfile(profile); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agents.write", profile.Name, map[string]any{"enabled": profile.IsEnabled()})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case http.MethodDelete:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if !s.mainRuntime.Config.DeleteAgentProfile(name) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agents.delete", name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAssistants(w http.ResponseWriter, r *http.Request) {
	s.handleAgents(w, r)
}
