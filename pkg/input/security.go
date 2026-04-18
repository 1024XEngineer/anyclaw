package input

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type DMPolicy string

const (
	DMPolicyAllowAll  DMPolicy = "allow-all"
	DMPolicyAllowList DMPolicy = "allow-list"
	DMPolicyPairing   DMPolicy = "pairing"
	DMPolicyDenyAll   DMPolicy = "deny-all"
)

type GroupPolicy string

const (
	GroupPolicyAllowAll  GroupPolicy = "allow-all"
	GroupPolicyAllowList GroupPolicy = "allow-list"
	GroupPolicyMention   GroupPolicy = "mention-only"
	GroupPolicyDenyAll   GroupPolicy = "deny-all"
)

type ChannelPolicy struct {
	mu               sync.RWMutex
	dmPolicy         DMPolicy
	groupPolicy      GroupPolicy
	allowFrom        map[string]bool
	pairingEnabled   bool
	pairingTTL       time.Duration
	mentionGate      bool
	riskAcknowledged bool
	defaultDenyDM    bool
}

func DefaultChannelPolicy() *ChannelPolicy {
	return &ChannelPolicy{
		dmPolicy:         DMPolicyAllowList,
		groupPolicy:      GroupPolicyMention,
		allowFrom:        make(map[string]bool),
		pairingEnabled:   false,
		pairingTTL:       72 * time.Hour,
		mentionGate:      true,
		riskAcknowledged: false,
		defaultDenyDM:    true,
	}
}

func ChannelPolicyFromConfig(cfg config.ChannelSecurityConfig) *ChannelPolicy {
	policy := DefaultChannelPolicy()

	dm := DMPolicy(strings.TrimSpace(cfg.DMPolicy))
	switch dm {
	case DMPolicyAllowAll, DMPolicyAllowList, DMPolicyPairing, DMPolicyDenyAll:
		policy.dmPolicy = dm
	default:
		if cfg.DMPolicy != "" {
			policy.dmPolicy = DMPolicy(cfg.DMPolicy)
		}
	}

	gp := GroupPolicy(strings.TrimSpace(cfg.GroupPolicy))
	switch gp {
	case GroupPolicyAllowAll, GroupPolicyAllowList, GroupPolicyMention, GroupPolicyDenyAll:
		policy.groupPolicy = gp
	default:
		if cfg.GroupPolicy != "" {
			policy.groupPolicy = GroupPolicy(cfg.GroupPolicy)
		}
	}

	policy.allowFrom = make(map[string]bool)
	for _, id := range cfg.AllowFrom {
		if id = strings.TrimSpace(id); id != "" {
			policy.allowFrom[id] = true
		}
	}

	if cfg.PairingEnabled {
		policy.pairingEnabled = true
	}
	if cfg.PairingTTLHours > 0 {
		policy.pairingTTL = time.Duration(cfg.PairingTTLHours) * time.Hour
	}
	if cfg.MentionGate {
		policy.mentionGate = true
	}
	policy.riskAcknowledged = cfg.RiskAcknowledged
	if cfg.DefaultDenyDM {
		policy.defaultDenyDM = true
	}

	return policy
}

func (p *ChannelPolicy) Validate() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var issues []string

	switch p.dmPolicy {
	case DMPolicyAllowAll, DMPolicyAllowList, DMPolicyPairing, DMPolicyDenyAll:
	default:
		issues = append(issues, fmt.Sprintf("invalid dm_policy: %q", p.dmPolicy))
	}

	switch p.groupPolicy {
	case GroupPolicyAllowAll, GroupPolicyAllowList, GroupPolicyMention, GroupPolicyDenyAll:
	default:
		issues = append(issues, fmt.Sprintf("invalid group_policy: %q", p.groupPolicy))
	}

	if p.dmPolicy == DMPolicyAllowList && len(p.allowFrom) == 0 {
		issues = append(issues, "dm_policy is allow-list but allow_from is empty")
	}

	if p.dmPolicy == DMPolicyAllowAll && !p.riskAcknowledged {
		issues = append(issues, "dm_policy is allow-all without risk acknowledgement")
	}

	if p.pairingTTL <= 0 {
		issues = append(issues, "pairing_ttl must be positive")
	}

	return issues
}

func (p *ChannelPolicy) AllowDM(userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch p.dmPolicy {
	case DMPolicyDenyAll:
		return false
	case DMPolicyAllowAll:
		return true
	case DMPolicyAllowList:
		return p.allowFrom[userID]
	case DMPolicyPairing:
		return true
	default:
		if p.defaultDenyDM {
			return false
		}
		return true
	}
}

func (p *ChannelPolicy) AllowGroup(userID, groupID string, mentioned bool) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch p.groupPolicy {
	case GroupPolicyDenyAll:
		return false
	case GroupPolicyAllowAll:
		return true
	case GroupPolicyAllowList:
		return p.allowFrom[userID]
	case GroupPolicyMention:
		return mentioned
	default:
		return mentioned
	}
}

func (p *ChannelPolicy) IsUserAllowed(userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.allowFrom) == 0 {
		return true
	}
	return p.allowFrom[userID]
}

func (p *ChannelPolicy) AddAllowedUser(userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	userID = strings.TrimSpace(userID)
	if userID != "" {
		p.allowFrom[userID] = true
	}
}

func (p *ChannelPolicy) RemoveAllowedUser(userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.allowFrom, strings.TrimSpace(userID))
}

func (p *ChannelPolicy) AllowedUsers() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]string, 0, len(p.allowFrom))
	for id := range p.allowFrom {
		result = append(result, id)
	}
	return result
}

func (p *ChannelPolicy) DMPolicy() DMPolicy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.dmPolicy
}

func (p *ChannelPolicy) SetDMPolicy(policy DMPolicy) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dmPolicy = policy
}

func (p *ChannelPolicy) GroupPolicy() GroupPolicy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.groupPolicy
}

func (p *ChannelPolicy) SetGroupPolicy(policy GroupPolicy) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.groupPolicy = policy
}

func (p *ChannelPolicy) MentionGateEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mentionGate
}

func (p *ChannelPolicy) SetMentionGate(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mentionGate = enabled
}

func (p *ChannelPolicy) PairingEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pairingEnabled
}

func (p *ChannelPolicy) SetPairingEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pairingEnabled = enabled
}

func (p *ChannelPolicy) PairingTTL() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pairingTTL
}

func (p *ChannelPolicy) SetPairingTTL(ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pairingTTL = ttl
}

func (p *ChannelPolicy) RiskAcknowledged() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.riskAcknowledged
}

func (p *ChannelPolicy) AcknowledgeRisk() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.riskAcknowledged = true
}

func (p *ChannelPolicy) DefaultDenyDM() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defaultDenyDM
}

func (p *ChannelPolicy) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		userID := meta["user_id"]
		channelType := meta["channel_type"]
		isGroup := meta["is_group"] == "true"
		isGuild := meta["guild_id"] != ""
		mentioned := isMentioned(message, []string{meta["bot_user_id"]})

		if isGroup || isGuild {
			if !p.AllowGroup(userID, meta["guild_id"], mentioned) {
				return sessionID, "", fmt.Errorf("user %s blocked by group policy", userID)
			}
		} else if channelType == "dm" || channelType == "private" {
			if !p.AllowDM(userID) {
				return sessionID, "", fmt.Errorf("user %s blocked by DM policy", userID)
			}
		}

		if len(p.allowFrom) > 0 && !p.IsUserAllowed(userID) {
			return sessionID, "", fmt.Errorf("user %s not in allow_from list", userID)
		}

		return handler(ctx, sessionID, message, meta)
	}
}

func (p *ChannelPolicy) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		userID := meta["user_id"]
		channelType := meta["channel_type"]
		isGroup := meta["is_group"] == "true"
		isGuild := meta["guild_id"] != ""
		mentioned := isMentioned(message, []string{meta["bot_user_id"]})

		if isGroup || isGuild {
			if !p.AllowGroup(userID, meta["guild_id"], mentioned) {
				return sessionID, fmt.Errorf("user %s blocked by group policy", userID)
			}
		} else if channelType == "dm" || channelType == "private" {
			if !p.AllowDM(userID) {
				return sessionID, fmt.Errorf("user %s blocked by DM policy", userID)
			}
		}

		if len(p.allowFrom) > 0 && !p.IsUserAllowed(userID) {
			return sessionID, fmt.Errorf("user %s not in allow_from list", userID)
		}

		return handler(ctx, sessionID, message, meta, onChunk)
	}
}

type SecurityAuditResult struct {
	Issues   []SecurityAuditIssue `json:"issues"`
	Passed   bool                 `json:"passed"`
	Score    int                  `json:"score"`
	MaxScore int                  `json:"max_score"`
}

type SecurityAuditIssue struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Fixable   bool   `json:"fixable"`
	FixAction string `json:"fix_action,omitempty"`
}

func AuditChannelPolicy(policy *ChannelPolicy) SecurityAuditResult {
	var issues []SecurityAuditIssue
	score := 0
	maxScore := 7

	policy.mu.RLock()
	dmPolicy := policy.dmPolicy
	groupPolicy := policy.groupPolicy
	allowFromCount := len(policy.allowFrom)
	mentionGate := policy.mentionGate
	riskAck := policy.riskAcknowledged
	defaultDeny := policy.defaultDenyDM
	policy.mu.RUnlock()
	_ = allowFromCount

	if dmPolicy != DMPolicyAllowAll || riskAck {
		score++
	} else {
		issues = append(issues, SecurityAuditIssue{
			ID:        "dm-allow-all-no-ack",
			Title:     "DM allow-all without risk acknowledgement",
			Severity:  "critical",
			Message:   "DM policy is allow-all but risk has not been acknowledged",
			Fixable:   true,
			FixAction: "set channels.security.risk_acknowledged=true or change dm_policy to allow-list",
		})
	}

	if dmPolicy == DMPolicyDenyAll || dmPolicy == DMPolicyAllowList || dmPolicy == DMPolicyPairing {
		score++
	} else {
		issues = append(issues, SecurityAuditIssue{
			ID:        "dm-policy-permissive",
			Title:     "DM policy is permissive",
			Severity:  "warning",
			Message:   fmt.Sprintf("DM policy is %q, consider allow-list or pairing", dmPolicy),
			Fixable:   true,
			FixAction: "set channels.security.dm_policy=allow-list",
		})
	}

	if mentionGate {
		score++
	} else {
		issues = append(issues, SecurityAuditIssue{
			ID:        "mention-gate-disabled",
			Title:     "Mention gate disabled",
			Severity:  "warning",
			Message:   "Group messages without @mention will be processed",
			Fixable:   true,
			FixAction: "set channels.security.mention_gate=true",
		})
	}

	if groupPolicy == GroupPolicyMention || groupPolicy == GroupPolicyAllowList || groupPolicy == GroupPolicyDenyAll {
		score++
	} else {
		issues = append(issues, SecurityAuditIssue{
			ID:        "group-policy-permissive",
			Title:     "Group policy is permissive",
			Severity:  "warning",
			Message:   fmt.Sprintf("Group policy is %q, consider mention-only", groupPolicy),
			Fixable:   true,
			FixAction: "set channels.security.group_policy=mention-only",
		})
	}

	if defaultDeny || dmPolicy != DMPolicyAllowAll {
		score++
	} else {
		issues = append(issues, SecurityAuditIssue{
			ID:        "no-default-deny",
			Title:     "No default-deny for DM",
			Severity:  "warning",
			Message:   "DM default-deny is not enabled",
			Fixable:   true,
			FixAction: "set channels.security.default_deny_dm=true",
		})
	}

	if riskAck {
		score++
	} else {
		issues = append(issues, SecurityAuditIssue{
			ID:        "risk-not-acknowledged",
			Title:     "Risk not acknowledged",
			Severity:  "info",
			Message:   "No explicit risk acknowledgement on file",
			Fixable:   true,
			FixAction: "set security.risk_acknowledged=true",
		})
	}

	if allowFromCount > 0 || dmPolicy == DMPolicyDenyAll {
		score++
	} else {
		issues = append(issues, SecurityAuditIssue{
			ID:        "no-allowlist",
			Title:     "No user allowlist configured",
			Severity:  "info",
			Message:   fmt.Sprintf("No users in allow_from list (%d entries)", allowFromCount),
			Fixable:   true,
			FixAction: "add user IDs to channels.security.allow_from",
		})
	}

	return SecurityAuditResult{
		Issues:   issues,
		Passed:   len(issues) == 0,
		Score:    score,
		MaxScore: maxScore,
	}
}
