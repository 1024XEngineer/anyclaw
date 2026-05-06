package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func (s *Server) handleConfigAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "config.read", "config", nil)
		writeJSON(w, http.StatusOK, s.configAPIView())
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		var cfg map[string]any
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		permissionChanged, err := s.applyAgentConfigPatch(cfg)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.applyLLMConfigPatch(cfg)
		if err := s.applyChannelRoutingPatch(cfg); err != nil {
			if duplicateErr, ok := err.(*duplicateRoutingRuleError); ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": duplicateErr.Error(), "details": duplicateErr.key})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if permissionChanged && s.mainRuntime != nil {
			if err := s.mainRuntime.RefreshToolRegistry(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		if permissionChanged && s.runtimePool != nil {
			s.runtimePool.InvalidateAll()
		}
		s.appendAudit(UserFromContext(r.Context()), "config.write", "config", nil)
		writeJSON(w, http.StatusOK, s.configAPIView())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type configReadView struct {
	Agent    configReadAgentView    `json:"agent"`
	LLM      configReadLLMView      `json:"llm"`
	Channels configReadChannelsView `json:"channels"`
}

type configReadAgentView struct {
	PermissionLevel string `json:"permission_level"`
}

type configReadLLMView struct {
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	DefaultProviderRef string `json:"default_provider_ref,omitempty"`
}

type configReadChannelsView struct {
	Routing config.RoutingConfig `json:"routing"`
}

func (s *Server) configAPIView() configReadView {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return configReadView{}
	}
	cfg := s.mainRuntime.Config
	return configReadView{
		Agent: configReadAgentView{
			PermissionLevel: cfg.Agent.PermissionLevel,
		},
		LLM: configReadLLMView{
			Provider:           cfg.LLM.Provider,
			Model:              cfg.LLM.Model,
			DefaultProviderRef: cfg.LLM.DefaultProviderRef,
		},
		Channels: configReadChannelsView{
			Routing: cloneRoutingConfig(cfg.Channels.Routing),
		},
	}
}

func (s *Server) applyLLMConfigPatch(cfg map[string]any) {
	llmCfg, ok := cfg["llm"].(map[string]any)
	if !ok {
		return
	}
	if provider, ok := llmCfg["provider"].(string); ok {
		s.mainRuntime.Config.LLM.Provider = provider
	}
	if model, ok := llmCfg["model"].(string); ok {
		s.mainRuntime.Config.LLM.Model = model
	}
}

func (s *Server) applyAgentConfigPatch(cfg map[string]any) (bool, error) {
	agentCfg, ok := cfg["agent"].(map[string]any)
	if !ok {
		return false, nil
	}
	permissionLevel, ok := agentCfg["permission_level"].(string)
	if !ok {
		return false, nil
	}
	permissionLevel = strings.TrimSpace(permissionLevel)
	if permissionLevel == "" {
		return false, nil
	}
	if !isValidPermissionLevel(permissionLevel) {
		return false, fmt.Errorf("agent.permission_level must be one of: full, limited, read-only (got %q)", permissionLevel)
	}
	profileChanged := false
	if profile, ok := s.mainRuntime.Config.ResolveMainAgentProfile(); ok {
		profileChanged = strings.TrimSpace(profile.PermissionLevel) != permissionLevel
		profile.PermissionLevel = permissionLevel
		if err := s.mainRuntime.Config.UpsertAgentProfile(profile); err != nil {
			return false, err
		}
	}
	configChanged := strings.TrimSpace(s.mainRuntime.Config.Agent.PermissionLevel) != permissionLevel
	s.mainRuntime.Config.Agent.PermissionLevel = permissionLevel
	return profileChanged || configChanged, nil
}

func isValidPermissionLevel(level string) bool {
	switch strings.TrimSpace(level) {
	case "full", "limited", "read-only":
		return true
	default:
		return false
	}
}

func (s *Server) applyChannelRoutingPatch(cfg map[string]any) error {
	channels, ok := cfg["channels"].(map[string]any)
	if !ok {
		return nil
	}
	routing, ok := channels["routing"].(map[string]any)
	if !ok {
		return nil
	}
	if mode, ok := routing["mode"].(string); ok {
		s.mainRuntime.Config.Channels.Routing.Mode = mode
	}
	rawRules, ok := routing["rules"].([]any)
	if !ok {
		return nil
	}
	rules, err := parseChannelRoutingRules(rawRules)
	if err != nil {
		return err
	}
	s.mainRuntime.Config.Channels.Routing.Rules = rules
	return nil
}

func parseChannelRoutingRules(rawRules []any) ([]config.ChannelRoutingRule, error) {
	rules := make([]config.ChannelRoutingRule, 0, len(rawRules))
	seen := map[string]bool{}
	for _, item := range rawRules {
		ruleMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rule := config.ChannelRoutingRule{}
		if v, ok := ruleMap["channel"].(string); ok {
			rule.Channel = v
		}
		if v, ok := ruleMap["match"].(string); ok {
			rule.Match = v
		}
		if v, ok := ruleMap["session_mode"].(string); ok {
			rule.SessionMode = v
		}
		if v, ok := ruleMap["session_id"].(string); ok {
			rule.SessionID = v
		}
		if v, ok := ruleMap["queue_mode"].(string); ok {
			rule.QueueMode = v
		}
		if v, ok := ruleMap["reply_back"].(bool); ok {
			replyBack := v
			rule.ReplyBack = &replyBack
		}
		if v, ok := ruleMap["title_prefix"].(string); ok {
			rule.TitlePrefix = v
		}
		if v, ok := ruleMap["agent"].(string); ok {
			rule.Agent = v
		}
		if v, ok := ruleMap["org"].(string); ok {
			rule.Org = v
		}
		if v, ok := ruleMap["project"].(string); ok {
			rule.Project = v
		}
		if v, ok := ruleMap["workspace"].(string); ok {
			rule.Workspace = v
		}
		if v, ok := ruleMap["workspace_ref"].(string); ok {
			rule.WorkspaceRef = v
		}
		conflictKey := strings.Join([]string{
			rule.Channel,
			rule.Match,
			rule.SessionMode,
			rule.SessionID,
			rule.QueueMode,
			rule.TitlePrefix,
			strconv.FormatBool(rule.ReplyBack != nil && *rule.ReplyBack),
		}, "|")
		if seen[conflictKey] {
			return nil, &duplicateRoutingRuleError{key: conflictKey}
		}
		seen[conflictKey] = true
		rules = append(rules, rule)
	}
	return rules, nil
}

func cloneRoutingConfig(routing config.RoutingConfig) config.RoutingConfig {
	rules := make([]config.ChannelRoutingRule, 0, len(routing.Rules))
	for _, rule := range routing.Rules {
		if rule.ReplyBack != nil {
			replyBack := *rule.ReplyBack
			rule.ReplyBack = &replyBack
		}
		rules = append(rules, rule)
	}
	routing.Rules = rules
	return routing
}

type duplicateRoutingRuleError struct {
	key string
}

func (e *duplicateRoutingRuleError) Error() string {
	return "duplicate routing rule"
}
