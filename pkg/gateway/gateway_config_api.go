package gateway

import (
	"encoding/json"
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
		writeJSON(w, http.StatusOK, s.mainRuntime.Config)
	case http.MethodPost:
		var cfg map[string]any
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
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
		s.appendAudit(UserFromContext(r.Context()), "config.write", "config", nil)
		writeJSON(w, http.StatusOK, s.mainRuntime.Config)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

type duplicateRoutingRuleError struct {
	key string
}

func (e *duplicateRoutingRuleError) Error() string {
	return "duplicate routing rule"
}
