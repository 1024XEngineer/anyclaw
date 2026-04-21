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
	pairedDMs        map[string]time.Time
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
		pairedDMs:        make(map[string]time.Time),
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
	if cfg.MentionGateSet() {
		policy.mentionGate = cfg.MentionGate
	}
	policy.riskAcknowledged = cfg.RiskAcknowledged
	if cfg.DefaultDenyDMSet() {
		policy.defaultDenyDM = cfg.DefaultDenyDM
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

	if p.dmPolicy == DMPolicyPairing && !p.pairingEnabled {
		issues = append(issues, "dm_policy is pairing but pairing is disabled")
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
	return p.allowDM(userID, nil)
}

func (p *ChannelPolicy) allowDM(userID string, meta map[string]string) bool {
	p.mu.RLock()
	dmPolicy := p.dmPolicy
	allowed := p.allowFrom[userID]
	pairingEnabled := p.pairingEnabled
	defaultDenyDM := p.defaultDenyDM
	p.mu.RUnlock()

	switch dmPolicy {
	case DMPolicyDenyAll:
		return false
	case DMPolicyAllowAll:
		return true
	case DMPolicyAllowList:
		return allowed
	case DMPolicyPairing:
		if !pairingEnabled {
			return false
		}
		return p.IsDMPaired(userID, meta)
	default:
		if defaultDenyDM {
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

func (p *ChannelPolicy) PairDM(userID string, meta map[string]string) {
	key := directPairingKey(userID, meta)
	if key == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.pairedDMs[key] = time.Now().UTC().Add(p.pairingTTL)
}

func (p *ChannelPolicy) UnpairDM(userID string, meta map[string]string) {
	key := directPairingKey(userID, meta)
	if key == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pairedDMs, key)
}

func (p *ChannelPolicy) IsDMPaired(userID string, meta map[string]string) bool {
	key := directPairingKey(userID, meta)
	if key == "" {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	expiresAt, ok := p.pairedDMs[key]
	if !ok {
		return false
	}
	if time.Now().UTC().After(expiresAt) {
		delete(p.pairedDMs, key)
		return false
	}
	return true
}

func (p *ChannelPolicy) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		userID := meta["user_id"]
		channelType, isGroup, groupID := inferChannelPolicyContext(meta)
		mentioned := isMentioned(message, mentionIDsForPolicy(meta))

		if isGroup {
			if !p.AllowGroup(userID, groupID, mentioned) {
				return sessionID, "", fmt.Errorf("user %s blocked by group policy", userID)
			}
		} else if isDirectChannelPolicyType(channelType) {
			if !p.allowDM(userID, meta) {
				return sessionID, "", fmt.Errorf("user %s blocked by DM policy", userID)
			}
		}

		return handler(ctx, sessionID, message, meta)
	}
}

func (p *ChannelPolicy) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		userID := meta["user_id"]
		channelType, isGroup, groupID := inferChannelPolicyContext(meta)
		mentioned := isMentioned(message, mentionIDsForPolicy(meta))

		if isGroup {
			if !p.AllowGroup(userID, groupID, mentioned) {
				return sessionID, fmt.Errorf("user %s blocked by group policy", userID)
			}
		} else if isDirectChannelPolicyType(channelType) {
			if !p.allowDM(userID, meta) {
				return sessionID, fmt.Errorf("user %s blocked by DM policy", userID)
			}
		}

		return handler(ctx, sessionID, message, meta, onChunk)
	}
}

func inferChannelPolicyContext(meta map[string]string) (string, bool, string) {
	channelType := strings.ToLower(strings.TrimSpace(meta["channel_type"]))
	isGroup := strings.EqualFold(strings.TrimSpace(meta["is_group"]), "true")
	groupID := strings.TrimSpace(meta["guild_id"])
	if groupID == "" {
		groupID = strings.TrimSpace(meta["thread_id"])
	}

	switch strings.ToLower(strings.TrimSpace(meta["channel"])) {
	case "discord":
		if channelType == "" {
			if strings.TrimSpace(meta["guild_id"]) != "" {
				channelType = "guild"
				isGroup = true
			} else {
				channelType = "private"
			}
		}
	case "slack":
		if channelType == "" {
			channelID := strings.ToUpper(strings.TrimSpace(meta["channel_id"]))
			if strings.HasPrefix(channelID, "D") {
				channelType = "dm"
			} else if channelID != "" {
				channelType = "group"
				isGroup = true
				if groupID == "" {
					groupID = channelID
				}
			}
		}
	case "telegram":
		chatID := strings.TrimSpace(meta["chat_id"])
		if channelType == "" {
			chatType := strings.ToLower(strings.TrimSpace(meta["chat_type"]))
			switch chatType {
			case "private":
				channelType = "private"
			case "group", "supergroup", "channel":
				channelType = chatType
				isGroup = true
			default:
				switch {
				case strings.HasPrefix(chatID, "-"):
					channelType = "group"
					isGroup = true
				case chatID != "" && chatID == strings.TrimSpace(meta["user_id"]):
					channelType = "private"
				}
			}
		}
		if groupID == "" && isGroup {
			groupID = chatID
		}
	case "signal":
		if channelType == "" {
			if strings.TrimSpace(meta["thread_id"]) != "" {
				channelType = "group"
				isGroup = true
			} else {
				channelType = "private"
			}
		}
	}

	if !isGroup {
		switch channelType {
		case "group", "supergroup", "guild", "channel":
			isGroup = true
		}
	}

	if groupID == "" && isGroup {
		if id := strings.TrimSpace(meta["chat_id"]); id != "" {
			groupID = id
		} else if id := strings.TrimSpace(meta["channel_id"]); id != "" {
			groupID = id
		}
	}

	if channelType == "" && !isGroup {
		channelType = "private"
	}

	return channelType, isGroup, groupID
}

func isDirectChannelPolicyType(channelType string) bool {
	switch channelType {
	case "dm", "private", "direct", "im":
		return true
	default:
		return false
	}
}

func mentionIDsForPolicy(meta map[string]string) []string {
	result := make([]string, 0, 4)
	if id := strings.TrimSpace(meta["bot_user_id"]); id != "" {
		result = append(result, id)
	}
	for _, rawID := range strings.Split(strings.TrimSpace(meta["bot_mention_ids"]), ",") {
		if id := strings.TrimSpace(rawID); id != "" {
			result = append(result, id)
		}
	}
	return result
}

func directPairingKey(userID string, meta map[string]string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}

	channel := strings.TrimSpace(meta["channel"])
	if channel == "" {
		channel = "direct"
	}

	scope := strings.TrimSpace(meta["device_id"])
	if scope == "" {
		scope = strings.TrimSpace(meta["chat_id"])
	}
	if scope == "" {
		scope = strings.TrimSpace(meta["channel_id"])
	}
	if scope == "" {
		scope = strings.TrimSpace(meta["thread_id"])
	}
	if scope == "" {
		scope = userID
	}

	return fmt.Sprintf("%s:%s:%s", channel, userID, scope)
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
