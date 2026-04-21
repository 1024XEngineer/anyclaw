package input

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ChannelCommands struct {
	mu       sync.RWMutex
	botName  string
	handlers map[string]CommandHandler
}

type CommandHandler func(ctx context.Context, args string, meta map[string]string) (string, error)

func NewChannelCommands(botName string) *ChannelCommands {
	cc := &ChannelCommands{
		botName:  botName,
		handlers: make(map[string]CommandHandler),
	}
	cc.Register("help", cc.handleHelp)
	cc.Register("status", cc.handleStatus)
	cc.Register("ping", cc.handlePing)
	cc.Register("sessions", cc.handleSessions)
	return cc
}

func (cc *ChannelCommands) Register(name string, handler CommandHandler) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.handlers[strings.ToLower(name)] = handler
}

func (cc *ChannelCommands) Handle(ctx context.Context, text string, meta map[string]string) (string, bool, error) {
	cmd, args := cc.parseCommand(text)
	if cmd == "" {
		return "", false, nil
	}

	cc.mu.RLock()
	handler, ok := cc.handlers[strings.ToLower(cmd)]
	cc.mu.RUnlock()

	if !ok {
		return fmt.Sprintf("Unknown command: /%s. Use /help for available commands.", cmd), true, nil
	}

	result, err := handler(ctx, args, meta)
	return result, true, err
}

func (cc *ChannelCommands) parseCommand(text string) (string, string) {
	text = strings.TrimSpace(text)

	if strings.HasPrefix(text, "/") {
		parts := strings.SplitN(text[1:], " ", 2)
		cmd := parts[0]
		args := ""
		if len(parts) > 1 {
			args = strings.TrimSpace(parts[1])
		}
		return cmd, args
	}

	botMentionPattern := fmt.Sprintf(`(?i)^@?%s\s*[/!](\w+)(.*)`, regexp.QuoteMeta(cc.botName))
	re := regexp.MustCompile(botMentionPattern)
	if matches := re.FindStringSubmatch(text); len(matches) >= 2 {
		cmd := matches[1]
		args := ""
		if len(matches) > 2 {
			args = strings.TrimSpace(matches[2])
		}
		return cmd, args
	}

	return "", ""
}

func (cc *ChannelCommands) handleHelp(ctx context.Context, args string, meta map[string]string) (string, error) {
	cc.mu.RLock()
	names := make([]string, 0, len(cc.handlers))
	for name := range cc.handlers {
		names = append(names, name)
	}
	cc.mu.RUnlock()

	helpText := "Available commands:\n"
	for _, name := range names {
		helpText += fmt.Sprintf("  /%s\n", name)
	}
	return helpText, nil
}

func (cc *ChannelCommands) handleStatus(ctx context.Context, args string, meta map[string]string) (string, error) {
	channel := meta["channel"]
	userID := meta["user_id"]
	username := meta["username"]
	if username == "" {
		username = meta["sender"]
	}

	status := fmt.Sprintf("AnyClaw Channel Status\n")
	status += fmt.Sprintf("Channel: %s\n", channel)
	if userID != "" {
		status += fmt.Sprintf("User: %s (%s)\n", username, userID)
	}
	status += fmt.Sprintf("Time: %s\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	status += "Status: Online"
	return status, nil
}

func (cc *ChannelCommands) handlePing(ctx context.Context, args string, meta map[string]string) (string, error) {
	return fmt.Sprintf("Pong! Latency: %dms", time.Now().UnixMilli()%1000), nil
}

func (cc *ChannelCommands) handleSessions(ctx context.Context, args string, meta map[string]string) (string, error) {
	return "Session management is available via the web UI.", nil
}

func (cc *ChannelCommands) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		result, handled, err := cc.Handle(ctx, message, meta)
		if handled {
			return sessionID, result, err
		}
		return handler(ctx, sessionID, message, meta)
	}
}

func (cc *ChannelCommands) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		result, handled, err := cc.Handle(ctx, message, meta)
		if handled {
			if err != nil {
				return sessionID, err
			}
			if strings.TrimSpace(result) != "" {
				if chunkErr := onChunk(result); chunkErr != nil {
					return sessionID, chunkErr
				}
			}
			return sessionID, nil
		}
		return handler(ctx, sessionID, message, meta, onChunk)
	}
}

type MentionGate struct {
	mu            sync.RWMutex
	enabled       bool
	botUserID     string
	botMentionIDs []string
}

func NewMentionGate(enabled bool, botUserID string, additionalMentions []string) *MentionGate {
	return &MentionGate{
		enabled:       enabled,
		botUserID:     botUserID,
		botMentionIDs: append([]string{botUserID}, additionalMentions...),
	}
}

func (mg *MentionGate) ShouldProcess(text string, meta map[string]string) bool {
	mg.mu.RLock()
	enabled := mg.enabled
	mentionIDs := mg.botMentionIDs
	mg.mu.RUnlock()

	if !enabled {
		return true
	}

	if isMentioned(text, mentionIDs) {
		return true
	}

	channelType, isGroup, _ := inferChannelPolicyContext(meta)
	if isDirectChannelPolicyType(channelType) {
		return true
	}

	if isGroup {
		return false
	}

	return true
}

func (mg *MentionGate) StripMention(text string) string {
	mg.mu.RLock()
	mentionIDs := mg.botMentionIDs
	mg.mu.RUnlock()

	return stripMentions(text, mentionIDs)
}

func (mg *MentionGate) SetEnabled(enabled bool) {
	mg.mu.Lock()
	mg.enabled = enabled
	mg.mu.Unlock()
}

func (mg *MentionGate) IsEnabled() bool {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	return mg.enabled
}

func (mg *MentionGate) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		mg.mu.RLock()
		enabled := mg.enabled
		botUserID := mg.botUserID
		mentionIDs := mg.botMentionIDs
		mg.mu.RUnlock()

		if !enabled {
			return handler(ctx, sessionID, message, meta)
		}

		channelType, isGroup, _ := inferChannelPolicyContext(meta)
		if isDirectChannelPolicyType(channelType) || !isGroup {
			return handler(ctx, sessionID, message, meta)
		}

		if !isMentioned(message, mentionIDs) {
			return sessionID, "", nil
		}

		cleanMessage := stripMentions(message, mentionIDs)
		if cleanMessage == "" {
			cleanMessage = message
		}
		meta["bot_user_id"] = botUserID
		meta["bot_mention_ids"] = strings.Join(mentionIDs, ",")
		return handler(ctx, sessionID, cleanMessage, meta)
	}
}

func (mg *MentionGate) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		mg.mu.RLock()
		enabled := mg.enabled
		botUserID := mg.botUserID
		mentionIDs := mg.botMentionIDs
		mg.mu.RUnlock()

		if !enabled {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		channelType, isGroup, _ := inferChannelPolicyContext(meta)
		if isDirectChannelPolicyType(channelType) || !isGroup {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		if !isMentioned(message, mentionIDs) {
			return sessionID, nil
		}

		cleanMessage := stripMentions(message, mentionIDs)
		if cleanMessage == "" {
			cleanMessage = message
		}
		meta["bot_user_id"] = botUserID
		meta["bot_mention_ids"] = strings.Join(mentionIDs, ",")
		return handler(ctx, sessionID, cleanMessage, meta, onChunk)
	}
}

func isMentioned(text string, mentionIDs []string) bool {
	for _, id := range mentionIDs {
		if id == "" {
			continue
		}
		discordMention := fmt.Sprintf("<@%s>", id)
		discordNickMention := fmt.Sprintf("<@!%s>", id)
		slackMention := fmt.Sprintf("<@%s>", id)

		if strings.Contains(text, discordMention) ||
			strings.Contains(text, discordNickMention) ||
			strings.Contains(text, slackMention) {
			return true
		}

		if strings.HasPrefix(strings.TrimSpace(text), "@"+id) {
			return true
		}
	}
	return false
}

func stripMentions(text string, mentionIDs []string) string {
	for _, id := range mentionIDs {
		if id == "" {
			continue
		}
		discordMention := fmt.Sprintf("<@%s> ", id)
		discordNickMention := fmt.Sprintf("<@!%s> ", id)
		slackMention := fmt.Sprintf("<@%s> ", id)

		text = strings.ReplaceAll(text, discordMention, "")
		text = strings.ReplaceAll(text, discordNickMention, "")
		text = strings.ReplaceAll(text, slackMention, "")

		discordMentionNoSpace := fmt.Sprintf("<@%s>", id)
		discordNickMentionNoSpace := fmt.Sprintf("<@!%s>", id)
		slackMentionNoSpace := fmt.Sprintf("<@%s>", id)

		text = strings.ReplaceAll(text, discordMentionNoSpace, "")
		text = strings.ReplaceAll(text, discordNickMentionNoSpace, "")
		text = strings.ReplaceAll(text, slackMentionNoSpace, "")
	}
	return strings.TrimSpace(text)
}

type GroupSecurity struct {
	mu              sync.RWMutex
	allowedGroups   map[string]map[string]bool
	deniedGroups    map[string]bool
	deniedUsers     map[string]bool
	allowedUsers    map[string]bool
	requireApproval bool
}

func NewGroupSecurity() *GroupSecurity {
	return &GroupSecurity{
		allowedGroups: make(map[string]map[string]bool),
		deniedGroups:  make(map[string]bool),
		deniedUsers:   make(map[string]bool),
		allowedUsers:  make(map[string]bool),
	}
}

func (gs *GroupSecurity) AllowGroup(groupID string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.allowedGroups[groupID] == nil {
		gs.allowedGroups[groupID] = make(map[string]bool)
	}
	gs.allowedGroups[groupID]["allowed"] = true
	delete(gs.deniedGroups, groupID)
}

func (gs *GroupSecurity) DenyGroup(groupID string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	delete(gs.allowedGroups, groupID)
	gs.deniedGroups[groupID] = true
}

func (gs *GroupSecurity) AllowUser(userID string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.allowedUsers[userID] = true
	delete(gs.deniedUsers, userID)
}

func (gs *GroupSecurity) DenyUser(userID string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.deniedUsers[userID] = true
	delete(gs.allowedUsers, userID)
}

func (gs *GroupSecurity) ShouldProcess(userID string, groupID string) bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	if gs.deniedUsers[userID] {
		return false
	}

	if gs.allowedUsers[userID] {
		return true
	}

	if groupID != "" {
		if gs.deniedGroups[groupID] {
			return false
		}
		if gs.allowedGroups[groupID] == nil {
			return !gs.requireApproval
		}
		return gs.allowedGroups[groupID]["allowed"]
	}

	return true
}

func (gs *GroupSecurity) SetRequireApproval(require bool) {
	gs.mu.Lock()
	gs.requireApproval = require
	gs.mu.Unlock()
}

func (gs *GroupSecurity) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		userID := meta["user_id"]
		groupID := meta["guild_id"]
		if groupID == "" {
			groupID = meta["chat_id"]
		}

		if !gs.ShouldProcess(userID, groupID) {
			return sessionID, "", fmt.Errorf("user %s not authorized in group %s", userID, groupID)
		}
		return handler(ctx, sessionID, message, meta)
	}
}

func (gs *GroupSecurity) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		userID := meta["user_id"]
		groupID := meta["guild_id"]
		if groupID == "" {
			groupID = meta["chat_id"]
		}

		if !gs.ShouldProcess(userID, groupID) {
			return sessionID, fmt.Errorf("user %s not authorized in group %s", userID, groupID)
		}
		return handler(ctx, sessionID, message, meta, onChunk)
	}
}

type ChannelPairing struct {
	mu       sync.RWMutex
	pairings map[string]PairingInfo
	enabled  bool
}

type PairingInfo struct {
	UserID      string    `json:"user_id"`
	DeviceID    string    `json:"device_id"`
	Channel     string    `json:"channel"`
	PairedAt    time.Time `json:"paired_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	LastSeen    time.Time `json:"last_seen"`
	DisplayName string    `json:"display_name"`
}

func NewChannelPairing() *ChannelPairing {
	return &ChannelPairing{
		pairings: make(map[string]PairingInfo),
	}
}

func (cp *ChannelPairing) Pair(userID, deviceID, channel, displayName string, ttl time.Duration) PairingInfo {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	key := cp.pairingKey(userID, deviceID, channel)
	now := time.Now().UTC()
	info := PairingInfo{
		UserID:      userID,
		DeviceID:    deviceID,
		Channel:     channel,
		PairedAt:    now,
		ExpiresAt:   now.Add(ttl),
		LastSeen:    now,
		DisplayName: displayName,
	}
	cp.pairings[key] = info
	return info
}

func (cp *ChannelPairing) Unpair(userID, deviceID, channel string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	key := cp.pairingKey(userID, deviceID, channel)
	delete(cp.pairings, key)
}

func (cp *ChannelPairing) IsPaired(userID, deviceID, channel string) bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	key := cp.pairingKey(userID, deviceID, channel)
	info, ok := cp.pairings[key]
	if !ok {
		return false
	}
	return time.Now().UTC().Before(info.ExpiresAt)
}

func (cp *ChannelPairing) UpdateLastSeen(userID, deviceID, channel string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	key := cp.pairingKey(userID, deviceID, channel)
	if info, ok := cp.pairings[key]; ok {
		info.LastSeen = time.Now().UTC()
		cp.pairings[key] = info
	}
}

func (cp *ChannelPairing) ListPaired() []PairingInfo {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	now := time.Now().UTC()
	result := make([]PairingInfo, 0, len(cp.pairings))
	for _, info := range cp.pairings {
		if now.Before(info.ExpiresAt) {
			result = append(result, info)
		}
	}
	return result
}

func (cp *ChannelPairing) CleanupExpired() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	now := time.Now().UTC()
	for key, info := range cp.pairings {
		if now.After(info.ExpiresAt) {
			delete(cp.pairings, key)
		}
	}
}

func (cp *ChannelPairing) pairingKey(userID, deviceID, channel string) string {
	return fmt.Sprintf("%s:%s:%s", channel, userID, deviceID)
}

func (cp *ChannelPairing) SetEnabled(enabled bool) {
	cp.mu.Lock()
	cp.enabled = enabled
	cp.mu.Unlock()
}

func (cp *ChannelPairing) IsEnabled() bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.enabled
}

func (cp *ChannelPairing) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		cp.mu.RLock()
		enabled := cp.enabled
		cp.mu.RUnlock()

		if !enabled {
			return handler(ctx, sessionID, message, meta)
		}

		userID := meta["user_id"]
		deviceID := meta["device_id"]
		channel := meta["channel"]

		if !cp.IsPaired(userID, deviceID, channel) {
			return sessionID, "Device not paired. Please pair your device first.", nil
		}

		cp.UpdateLastSeen(userID, deviceID, channel)
		return handler(ctx, sessionID, message, meta)
	}
}

func (cp *ChannelPairing) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		cp.mu.RLock()
		enabled := cp.enabled
		cp.mu.RUnlock()

		if !enabled {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		userID := meta["user_id"]
		deviceID := meta["device_id"]
		channel := meta["channel"]

		if !cp.IsPaired(userID, deviceID, channel) {
			return sessionID, fmt.Errorf("device not paired")
		}

		cp.UpdateLastSeen(userID, deviceID, channel)
		return handler(ctx, sessionID, message, meta, onChunk)
	}
}

type PresenceManager struct {
	mu        sync.RWMutex
	presences map[string]PresenceInfo
	onUpdate  func(channel, userID string, presence PresenceInfo)
}

type PresenceInfo struct {
	Status     string    `json:"status"`
	Activity   string    `json:"activity,omitempty"`
	Since      time.Time `json:"since"`
	LastUpdate time.Time `json:"last_update"`
}

func NewPresenceManager(onUpdate func(channel, userID string, presence PresenceInfo)) *PresenceManager {
	return &PresenceManager{
		presences: make(map[string]PresenceInfo),
		onUpdate:  onUpdate,
	}
}

func (pm *PresenceManager) SetPresence(channel, userID string, status string, activity string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	key := fmt.Sprintf("%s:%s", channel, userID)
	now := time.Now().UTC()
	info := PresenceInfo{
		Status:     status,
		Activity:   activity,
		Since:      now,
		LastUpdate: now,
	}

	if existing, ok := pm.presences[key]; ok && existing.Status == status {
		info.Since = existing.Since
	}

	pm.presences[key] = info

	if pm.onUpdate != nil {
		pm.onUpdate(channel, userID, info)
	}
}

func (pm *PresenceManager) GetPresence(channel, userID string) (PresenceInfo, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", channel, userID)
	info, ok := pm.presences[key]
	return info, ok
}

func (pm *PresenceManager) SetTyping(channel, userID string, typing bool) {
	if typing {
		pm.SetPresence(channel, userID, "typing", "")
	} else {
		pm.SetPresence(channel, userID, "online", "")
	}
}

func (pm *PresenceManager) SetOffline(channel, userID string) {
	pm.SetPresence(channel, userID, "offline", "")
}

func (pm *PresenceManager) ListActive() map[string]PresenceInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make(map[string]PresenceInfo)
	for k, v := range pm.presences {
		if v.Status != "offline" {
			result[k] = v
		}
	}
	return result
}

func (pm *PresenceManager) CleanupStale(timeout time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	now := time.Now().UTC()
	for key, info := range pm.presences {
		if now.Sub(info.LastUpdate) <= timeout {
			continue
		}
		if info.Status == "offline" {
			delete(pm.presences, key)
			continue
		}
		if pm.onUpdate != nil {
			channel, userID := splitPresenceKey(key)
			info.Status = "offline"
			info.Activity = ""
			info.LastUpdate = now
			pm.presences[key] = info
			pm.onUpdate(channel, userID, info)
			continue
		}
		info.Status = "offline"
		info.Activity = ""
		info.LastUpdate = now
		pm.presences[key] = info
	}
}

func splitPresenceKey(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func (pm *PresenceManager) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		channel := meta["channel"]
		userID := meta["user_id"]
		if channel != "" && userID != "" {
			pm.SetTyping(channel, userID, true)
		}

		respSessionID, response, err := handler(ctx, sessionID, message, meta)

		if channel != "" && userID != "" {
			pm.SetTyping(channel, userID, false)
		}
		return respSessionID, response, err
	}
}

func (pm *PresenceManager) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		channel := meta["channel"]
		userID := meta["user_id"]
		if channel != "" && userID != "" {
			pm.SetTyping(channel, userID, true)
		}

		respSessionID, err := handler(ctx, sessionID, message, meta, onChunk)

		if channel != "" && userID != "" {
			pm.SetTyping(channel, userID, false)
		}
		return respSessionID, err
	}
}

type ContactDirectory struct {
	mu         sync.RWMutex
	contacts   map[string]ContactInfo
	maxEntries int
	staleAfter time.Duration
}

type ContactInfo struct {
	UserID      string            `json:"user_id"`
	Channel     string            `json:"channel"`
	DisplayName string            `json:"display_name"`
	Username    string            `json:"username"`
	FirstName   string            `json:"first_name"`
	LastName    string            `json:"last_name"`
	IsBot       bool              `json:"is_bot"`
	AddedAt     time.Time         `json:"added_at"`
	LastSeen    time.Time         `json:"last_seen"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func NewContactDirectory() *ContactDirectory {
	return &ContactDirectory{
		contacts:   make(map[string]ContactInfo),
		maxEntries: 4096,
		staleAfter: 30 * 24 * time.Hour,
	}
}

func (cd *ContactDirectory) AddOrUpdate(contact ContactInfo) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	key := fmt.Sprintf("%s:%s", contact.Channel, contact.UserID)
	now := time.Now().UTC()

	if existing, ok := cd.contacts[key]; ok {
		contact.AddedAt = existing.AddedAt
		contact.LastSeen = now
		if contact.DisplayName == "" {
			contact.DisplayName = existing.DisplayName
		}
		if contact.Username == "" {
			contact.Username = existing.Username
		}
	} else {
		contact.AddedAt = now
		contact.LastSeen = now
	}

	cd.contacts[key] = contact
	cd.cleanupStaleLocked(now, cd.staleAfter)
	cd.evictOverflowLocked()
}

func (cd *ContactDirectory) Get(channel, userID string) (ContactInfo, bool) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", channel, userID)
	contact, ok := cd.contacts[key]
	return contact, ok
}

func (cd *ContactDirectory) Remove(channel, userID string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	key := fmt.Sprintf("%s:%s", channel, userID)
	delete(cd.contacts, key)
}

func (cd *ContactDirectory) List(channel string) []ContactInfo {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	result := make([]ContactInfo, 0, len(cd.contacts))
	for _, contact := range cd.contacts {
		if channel == "" || contact.Channel == channel {
			result = append(result, contact)
		}
	}
	return result
}

func (cd *ContactDirectory) Search(query string) []ContactInfo {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	query = strings.ToLower(query)
	result := make([]ContactInfo, 0, len(cd.contacts))
	for _, contact := range cd.contacts {
		if strings.Contains(strings.ToLower(contact.DisplayName), query) ||
			strings.Contains(strings.ToLower(contact.Username), query) ||
			strings.Contains(strings.ToLower(contact.UserID), query) {
			result = append(result, contact)
		}
	}
	return result
}

func (cd *ContactDirectory) Count() int {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return len(cd.contacts)
}

func (cd *ContactDirectory) SetMaxEntries(maxEntries int) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.maxEntries = maxEntries
	cd.evictOverflowLocked()
}

func (cd *ContactDirectory) SetStaleAfter(staleAfter time.Duration) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.staleAfter = staleAfter
	cd.cleanupStaleLocked(time.Now().UTC(), cd.staleAfter)
}

func (cd *ContactDirectory) CleanupStale(maxAge time.Duration) int {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.cleanupStaleLocked(time.Now().UTC(), maxAge)
}

func (cd *ContactDirectory) cleanupStaleLocked(now time.Time, maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}

	removed := 0
	for key, contact := range cd.contacts {
		if now.Sub(contact.LastSeen) > maxAge {
			delete(cd.contacts, key)
			removed++
		}
	}
	return removed
}

func (cd *ContactDirectory) evictOverflowLocked() int {
	if cd.maxEntries <= 0 || len(cd.contacts) <= cd.maxEntries {
		return 0
	}

	removed := 0
	for len(cd.contacts) > cd.maxEntries {
		var oldestKey string
		var oldestSeen time.Time
		first := true
		for key, contact := range cd.contacts {
			if first || contact.LastSeen.Before(oldestSeen) {
				oldestKey = key
				oldestSeen = contact.LastSeen
				first = false
			}
		}
		if first {
			return removed
		}
		delete(cd.contacts, oldestKey)
		removed++
	}
	return removed
}

func (cd *ContactDirectory) Wrap(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		userID := meta["user_id"]
		channel := meta["channel"]
		if userID != "" && channel != "" {
			cd.AddOrUpdate(ContactInfo{
				UserID:      userID,
				Channel:     channel,
				DisplayName: meta["username"],
				Username:    meta["username"],
				Metadata:    meta,
			})
		}
		return handler(ctx, sessionID, message, meta)
	}
}

func (cd *ContactDirectory) WrapStream(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		userID := meta["user_id"]
		channel := meta["channel"]
		if userID != "" && channel != "" {
			cd.AddOrUpdate(ContactInfo{
				UserID:      userID,
				Channel:     channel,
				DisplayName: meta["username"],
				Username:    meta["username"],
				Metadata:    meta,
			})
		}
		return handler(ctx, sessionID, message, meta, onChunk)
	}
}
