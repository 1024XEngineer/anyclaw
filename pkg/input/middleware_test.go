package input

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestChannelCommandsWrapStreamEmitsCommandOutput(t *testing.T) {
	cc := NewChannelCommands("AnyClaw")
	wrapped := cc.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		t.Fatal("stream handler should not be called for built-in commands")
		return "", nil
	})

	var chunks []string
	sessionID, err := wrapped(context.Background(), "session-1", "/help", map[string]string{"channel": "slack"}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("wrap stream returned error: %v", err)
	}
	if sessionID != "session-1" {
		t.Fatalf("expected session to be preserved, got %q", sessionID)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected one streamed command response, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0], "Available commands:") {
		t.Fatalf("expected streamed help output, got %q", chunks[0])
	}
}

func TestMentionGateBlocksSlackChannelMessagesWithoutMentionFallback(t *testing.T) {
	gate := NewMentionGate(true, "BOT123", nil)

	if gate.ShouldProcess("hello team", map[string]string{
		"channel":    "slack",
		"channel_id": "C123456",
	}) {
		t.Fatal("expected slack channel message without mention to be blocked")
	}
}

func TestMentionGateBlocksTelegramGroupMessagesWithoutMentionFallback(t *testing.T) {
	gate := NewMentionGate(true, "bot", nil)

	if gate.ShouldProcess("hello group", map[string]string{
		"channel":   "telegram",
		"chat_id":   "-100123",
		"chat_type": "supergroup",
	}) {
		t.Fatal("expected telegram group message without mention to be blocked")
	}
}

func TestGroupSecurityDenyGroupBlocksWithoutApprovalMode(t *testing.T) {
	security := NewGroupSecurity()
	security.DenyGroup("group-1")

	if security.ShouldProcess("user-1", "group-1") {
		t.Fatal("expected denied group to be blocked even when approval mode is off")
	}
}

func TestChannelCommandsHandleBuiltinsAndWrap(t *testing.T) {
	cc := NewChannelCommands("AnyClaw")

	if result, handled, err := cc.Handle(context.Background(), "hello", nil); err != nil || handled || result != "" {
		t.Fatalf("expected plain text to bypass command handling, got result=%q handled=%v err=%v", result, handled, err)
	}

	result, handled, err := cc.Handle(context.Background(), "/unknown", nil)
	if err != nil {
		t.Fatalf("unknown command returned error: %v", err)
	}
	if !handled || !strings.Contains(result, "Unknown command: /unknown") {
		t.Fatalf("expected unknown command message, got handled=%v result=%q", handled, result)
	}

	result, handled, err = cc.Handle(context.Background(), "/status", map[string]string{
		"channel":  "slack",
		"user_id":  "U123",
		"username": "alice",
	})
	if err != nil || !handled {
		t.Fatalf("status command failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(result, "AnyClaw Channel Status") ||
		!strings.Contains(result, "Channel: slack") ||
		!strings.Contains(result, "User: alice (U123)") ||
		!strings.Contains(result, "Status: Online") {
		t.Fatalf("unexpected status output: %q", result)
	}

	result, handled, err = cc.Handle(context.Background(), "@AnyClaw !ping", nil)
	if err != nil || !handled {
		t.Fatalf("ping command failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(result, "Pong! Latency:") {
		t.Fatalf("unexpected ping output: %q", result)
	}

	result, handled, err = cc.Handle(context.Background(), "/sessions", nil)
	if err != nil || !handled {
		t.Fatalf("sessions command failed: handled=%v err=%v", handled, err)
	}
	if result != "Session management is available via the web UI." {
		t.Fatalf("unexpected sessions output: %q", result)
	}

	fellThrough := false
	wrapped := cc.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		fellThrough = true
		return sessionID, "fallback", nil
	})

	sessionID, response, err := wrapped(context.Background(), "session-1", "hello", map[string]string{"channel": "slack"})
	if err != nil {
		t.Fatalf("wrapped fallback returned error: %v", err)
	}
	if !fellThrough || sessionID != "session-1" || response != "fallback" {
		t.Fatalf("expected fallback handler to run, got fellThrough=%v session=%q response=%q", fellThrough, sessionID, response)
	}

	commandSession, commandResponse, err := wrapped(context.Background(), "session-2", "/help", map[string]string{"channel": "slack"})
	if err != nil {
		t.Fatalf("expected wrapped command to succeed: %v", err)
	}
	if commandSession != "session-2" || !strings.Contains(commandResponse, "Available commands:") {
		t.Fatalf("unexpected wrapped command response: session=%q response=%q", commandSession, commandResponse)
	}
}

func TestMentionGateHelpersAndWrap(t *testing.T) {
	groupMeta := map[string]string{
		"channel":    "slack",
		"channel_id": "C123456",
	}
	dmMeta := map[string]string{
		"channel":    "slack",
		"channel_id": "D123456",
	}

	gate := NewMentionGate(true, "BOT123", []string{"anyclaw"})
	if !gate.ShouldProcess("<@BOT123> hello", groupMeta) {
		t.Fatal("expected explicit mention to be processed")
	}
	if gate.ShouldProcess("hello team", groupMeta) {
		t.Fatal("expected group message without mention to be blocked")
	}
	if !gate.ShouldProcess("hello privately", dmMeta) {
		t.Fatal("expected direct message to bypass mention gate")
	}

	if stripped := gate.StripMention("<@BOT123> hello"); stripped != "hello" {
		t.Fatalf("expected mention to be stripped, got %q", stripped)
	}

	gate.SetEnabled(false)
	if gate.IsEnabled() {
		t.Fatal("expected mention gate to be disabled")
	}
	if !gate.ShouldProcess("hello team", groupMeta) {
		t.Fatal("expected disabled mention gate to allow message")
	}

	gate.SetEnabled(true)
	wrappedMeta := map[string]string{
		"channel":    "slack",
		"channel_id": "C123456",
	}
	called := false
	wrapped := gate.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		called = true
		if message != "hello" {
			t.Fatalf("expected cleaned message, got %q", message)
		}
		if meta["bot_user_id"] != "BOT123" {
			t.Fatalf("expected bot_user_id to be injected, got %q", meta["bot_user_id"])
		}
		if !strings.Contains(meta["bot_mention_ids"], "BOT123") {
			t.Fatalf("expected bot_mention_ids to include BOT123, got %q", meta["bot_mention_ids"])
		}
		return sessionID, "ok", nil
	})

	sessionID, response, err := wrapped(context.Background(), "session-1", "<@BOT123> hello", wrappedMeta)
	if err != nil {
		t.Fatalf("wrapped mention gate returned error: %v", err)
	}
	if !called || sessionID != "session-1" || response != "ok" {
		t.Fatalf("expected wrapped handler to succeed, got called=%v session=%q response=%q", called, sessionID, response)
	}

	called = false
	sessionID, response, err = wrapped(context.Background(), "session-2", "hello", map[string]string{
		"channel":    "slack",
		"channel_id": "C123456",
	})
	if err != nil {
		t.Fatalf("blocked mention gate returned error: %v", err)
	}
	if called || sessionID != "session-2" || response != "" {
		t.Fatalf("expected unmentioned group message to be dropped, got called=%v session=%q response=%q", called, sessionID, response)
	}
}

func TestMentionGateWrapStreamPassesCleanMessage(t *testing.T) {
	gate := NewMentionGate(true, "BOT123", nil)
	called := false
	wrapped := gate.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		called = true
		if message != "hello stream" {
			t.Fatalf("expected cleaned stream message, got %q", message)
		}
		return sessionID, onChunk("ok")
	})

	var chunks []string
	sessionID, err := wrapped(context.Background(), "session-1", "<@BOT123> hello stream", map[string]string{
		"channel":    "slack",
		"channel_id": "C123456",
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("wrapped stream returned error: %v", err)
	}
	if !called || sessionID != "session-1" || len(chunks) != 1 || chunks[0] != "ok" {
		t.Fatalf("unexpected wrapped stream result: called=%v session=%q chunks=%v", called, sessionID, chunks)
	}

	called = false
	sessionID, err = wrapped(context.Background(), "session-2", "ignored", map[string]string{
		"channel":    "slack",
		"channel_id": "C123456",
	}, func(chunk string) error {
		t.Fatalf("no chunk should be emitted for dropped stream message")
		return nil
	})
	if err != nil {
		t.Fatalf("blocked wrapped stream returned error: %v", err)
	}
	if called || sessionID != "session-2" {
		t.Fatalf("expected blocked stream message to be dropped, got called=%v session=%q", called, sessionID)
	}

	directCalled := false
	sessionID, err = gate.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		directCalled = true
		return sessionID, nil
	})(context.Background(), "session-3", "plain dm", map[string]string{
		"channel":    "slack",
		"channel_id": "D123456",
	}, func(chunk string) error { return nil })
	if err != nil {
		t.Fatalf("expected direct message stream to bypass mention gate: %v", err)
	}
	if !directCalled || sessionID != "session-3" {
		t.Fatalf("expected direct stream to reach handler, got called=%v session=%q", directCalled, sessionID)
	}
}

func TestGroupSecurityRulesAndWrappers(t *testing.T) {
	security := NewGroupSecurity()
	if !security.ShouldProcess("user-1", "group-1") {
		t.Fatal("expected group to be allowed before approval mode is enabled")
	}

	security.SetRequireApproval(true)
	if security.ShouldProcess("user-1", "group-1") {
		t.Fatal("expected unknown group to be blocked when approval is required")
	}

	security.AllowGroup("group-1")
	if !security.ShouldProcess("user-1", "group-1") {
		t.Fatal("expected approved group to be allowed")
	}

	security.DenyGroup("group-1")
	if security.ShouldProcess("user-1", "group-1") {
		t.Fatal("expected denied group to be blocked")
	}

	security.AllowUser("user-1")
	if !security.ShouldProcess("user-1", "group-1") {
		t.Fatal("expected explicitly allowed user to bypass group denial")
	}

	security.DenyUser("user-1")
	if security.ShouldProcess("user-1", "group-1") {
		t.Fatal("expected denied user to be blocked")
	}

	wrapped := security.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		return sessionID, "ok", nil
	})
	if _, _, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"user_id":  "user-1",
		"guild_id": "group-1",
	}); err == nil {
		t.Fatal("expected wrapped handler to block denied user")
	}

	security.AllowUser("user-2")
	sessionID, response, err := wrapped(context.Background(), "session-2", "hello", map[string]string{
		"user_id":  "user-2",
		"guild_id": "group-1",
	})
	if err != nil {
		t.Fatalf("expected allowed user to pass wrapped handler, got %v", err)
	}
	if sessionID != "session-2" || response != "ok" {
		t.Fatalf("unexpected wrapped response: session=%q response=%q", sessionID, response)
	}

	wrappedStream := security.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		return sessionID, onChunk("ok")
	})
	var chunks []string
	sessionID, err = wrappedStream(context.Background(), "session-3", "hello", map[string]string{
		"user_id":  "user-2",
		"guild_id": "group-1",
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("expected allowed user to pass wrapped stream, got %v", err)
	}
	if sessionID != "session-3" || len(chunks) != 1 || chunks[0] != "ok" {
		t.Fatalf("unexpected wrapped stream result: session=%q chunks=%v", sessionID, chunks)
	}

	if _, err := wrappedStream(context.Background(), "session-4", "hello", map[string]string{
		"user_id":  "user-1",
		"guild_id": "group-1",
	}, func(chunk string) error { return nil }); err == nil {
		t.Fatal("expected denied stream user to be blocked")
	}
}

func TestChannelPairingLifecycleAndWrappers(t *testing.T) {
	pairing := NewChannelPairing()
	if pairing.IsEnabled() {
		t.Fatal("expected channel pairing to be disabled by default")
	}
	pairing.SetEnabled(true)
	if !pairing.IsEnabled() {
		t.Fatal("expected channel pairing to be enabled")
	}

	info := pairing.Pair("user-1", "device-1", "slack", "Alice", time.Hour)
	if info.UserID != "user-1" || info.DeviceID != "device-1" || info.Channel != "slack" || info.DisplayName != "Alice" {
		t.Fatalf("unexpected pairing info: %+v", info)
	}
	if !pairing.IsPaired("user-1", "device-1", "slack") {
		t.Fatal("expected paired device to be recognized")
	}
	if len(pairing.ListPaired()) != 1 {
		t.Fatalf("expected one active pairing, got %d", len(pairing.ListPaired()))
	}

	key := pairing.pairingKey("user-1", "device-1", "slack")
	before := pairing.pairings[key].LastSeen
	pairing.UpdateLastSeen("user-1", "device-1", "slack")
	after := pairing.pairings[key].LastSeen
	if after.Before(before) {
		t.Fatalf("expected last seen to move forward, before=%v after=%v", before, after)
	}

	wrapped := pairing.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		return sessionID, "ok", nil
	})
	sessionID, response, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"channel":   "slack",
		"user_id":   "user-2",
		"device_id": "device-2",
	})
	if err != nil {
		t.Fatalf("unpaired wrapper should return guidance instead of error: %v", err)
	}
	if sessionID != "session-1" || !strings.Contains(response, "Device not paired") {
		t.Fatalf("unexpected unpaired response: session=%q response=%q", sessionID, response)
	}

	sessionID, response, err = wrapped(context.Background(), "session-2", "hello", map[string]string{
		"channel":   "slack",
		"user_id":   "user-1",
		"device_id": "device-1",
	})
	if err != nil {
		t.Fatalf("expected paired wrapper to succeed: %v", err)
	}
	if sessionID != "session-2" || response != "ok" {
		t.Fatalf("unexpected paired wrapper result: session=%q response=%q", sessionID, response)
	}

	wrappedStream := pairing.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		return sessionID, onChunk("stream-ok")
	})
	if _, err := wrappedStream(context.Background(), "session-3", "hello", map[string]string{
		"channel":   "slack",
		"user_id":   "user-2",
		"device_id": "device-2",
	}, func(chunk string) error { return nil }); err == nil {
		t.Fatal("expected unpaired stream wrapper to return error")
	}

	var chunks []string
	sessionID, err = wrappedStream(context.Background(), "session-4", "hello", map[string]string{
		"channel":   "slack",
		"user_id":   "user-1",
		"device_id": "device-1",
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("expected paired stream wrapper to succeed: %v", err)
	}
	if sessionID != "session-4" || len(chunks) != 1 || chunks[0] != "stream-ok" {
		t.Fatalf("unexpected paired stream result: session=%q chunks=%v", sessionID, chunks)
	}

	pairing.pairings[key] = PairingInfo{
		UserID:    "user-1",
		DeviceID:  "device-1",
		Channel:   "slack",
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	}
	pairing.CleanupExpired()
	if pairing.IsPaired("user-1", "device-1", "slack") {
		t.Fatal("expected expired pairing to be removed")
	}

	pairing.Unpair("user-1", "device-1", "slack")
	if got := pairing.pairingKey("user-1", "device-1", "slack"); got != "slack:user-1:device-1" {
		t.Fatalf("unexpected pairing key: %q", got)
	}

	disabled := NewChannelPairing()
	bypassed := false
	_, response, err = disabled.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		bypassed = true
		return sessionID, "ok", nil
	})(context.Background(), "session-5", "hello", map[string]string{
		"channel":   "slack",
		"user_id":   "user-x",
		"device_id": "device-x",
	})
	if err != nil {
		t.Fatalf("expected disabled pairing wrapper to bypass checks: %v", err)
	}
	if !bypassed || response != "ok" {
		t.Fatalf("expected disabled pairing wrapper to call handler, got bypassed=%v response=%q", bypassed, response)
	}
}

func TestPresenceManagerLifecycleAndWrappers(t *testing.T) {
	var updates []string
	presence := NewPresenceManager(func(channel, userID string, info PresenceInfo) {
		updates = append(updates, info.Status)
	})

	referenceTime := time.Now().UTC().Add(-time.Hour)
	presence.presences["slack:user-1"] = PresenceInfo{
		Status:     "online",
		Activity:   "reading",
		Since:      referenceTime,
		LastUpdate: referenceTime,
	}

	presence.SetPresence("slack", "user-1", "online", "reading")
	info, ok := presence.GetPresence("slack", "user-1")
	if !ok || info.Status != "online" || info.Activity != "reading" {
		t.Fatalf("unexpected presence after SetPresence: ok=%v info=%+v", ok, info)
	}
	if !info.Since.Equal(referenceTime) {
		t.Fatalf("expected same-status presence to keep original since, got %v want %v", info.Since, referenceTime)
	}

	presence.SetTyping("slack", "user-1", true)
	info, ok = presence.GetPresence("slack", "user-1")
	if !ok || info.Status != "typing" {
		t.Fatalf("expected typing presence, got ok=%v info=%+v", ok, info)
	}
	if !info.Since.After(referenceTime) {
		t.Fatalf("expected status change to reset since, got %v want after %v", info.Since, referenceTime)
	}

	presence.SetTyping("slack", "user-1", false)
	presence.SetOffline("slack", "user-1")
	if active := presence.ListActive(); len(active) != 0 {
		t.Fatalf("expected offline user to be excluded from active list, got %v", active)
	}

	staleKey := "slack:user-2"
	presence.presences[staleKey] = PresenceInfo{
		Status:     "online",
		Since:      time.Now().UTC().Add(-2 * time.Hour),
		LastUpdate: time.Now().UTC().Add(-2 * time.Hour),
	}
	presence.CleanupStale(time.Minute)
	staleInfo, ok := presence.GetPresence("slack", "user-2")
	if !ok || staleInfo.Status != "offline" {
		t.Fatalf("expected stale presence to be marked offline, got ok=%v info=%+v", ok, staleInfo)
	}
	presence.presences[staleKey] = PresenceInfo{
		Status:     "offline",
		Since:      time.Now().UTC().Add(-2 * time.Hour),
		LastUpdate: time.Now().UTC().Add(-2 * time.Hour),
	}
	presence.CleanupStale(time.Minute)
	if _, ok := presence.GetPresence("slack", "user-2"); ok {
		t.Fatal("expected stale offline presence to be removed")
	}

	wrappedPresence := NewPresenceManager(nil)
	wrapped := wrappedPresence.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		current, ok := wrappedPresence.GetPresence("slack", "user-3")
		if !ok || current.Status != "typing" {
			t.Fatalf("expected user to be typing during wrapped call, got ok=%v info=%+v", ok, current)
		}
		return sessionID, "ok", nil
	})
	sessionID, response, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"channel": "slack",
		"user_id": "user-3",
	})
	if err != nil {
		t.Fatalf("presence wrapper returned error: %v", err)
	}
	if sessionID != "session-1" || response != "ok" {
		t.Fatalf("unexpected presence wrapper result: session=%q response=%q", sessionID, response)
	}
	current, ok := wrappedPresence.GetPresence("slack", "user-3")
	if !ok || current.Status != "online" {
		t.Fatalf("expected wrapped presence to return to online, got ok=%v info=%+v", ok, current)
	}

	wrappedStreamPresence := NewPresenceManager(nil)
	wrappedStream := wrappedStreamPresence.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		current, ok := wrappedStreamPresence.GetPresence("slack", "user-4")
		if !ok || current.Status != "typing" {
			t.Fatalf("expected stream user to be typing during wrapped call, got ok=%v info=%+v", ok, current)
		}
		return sessionID, onChunk("ok")
	})
	var chunks []string
	sessionID, err = wrappedStream(context.Background(), "session-2", "hello", map[string]string{
		"channel": "slack",
		"user_id": "user-4",
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("presence stream wrapper returned error: %v", err)
	}
	if sessionID != "session-2" || len(chunks) != 1 || chunks[0] != "ok" {
		t.Fatalf("unexpected presence stream result: session=%q chunks=%v", sessionID, chunks)
	}
	current, ok = wrappedStreamPresence.GetPresence("slack", "user-4")
	if !ok || current.Status != "online" {
		t.Fatalf("expected stream presence to return to online, got ok=%v info=%+v", ok, current)
	}

	if len(updates) < 4 {
		t.Fatalf("expected update callback to be invoked multiple times, got %v", updates)
	}
}

func TestContactDirectoryLifecycleAndWrappers(t *testing.T) {
	directory := NewContactDirectory()
	directory.AddOrUpdate(ContactInfo{
		UserID:      "user-1",
		Channel:     "slack",
		DisplayName: "Alice",
		Username:    "alice",
		Metadata:    map[string]string{"role": "dev"},
	})

	first, ok := directory.Get("slack", "user-1")
	if !ok || first.DisplayName != "Alice" || first.Username != "alice" {
		t.Fatalf("unexpected first contact: ok=%v contact=%+v", ok, first)
	}

	directory.AddOrUpdate(ContactInfo{
		UserID:   "user-1",
		Channel:  "slack",
		Metadata: map[string]string{"role": "updated"},
	})
	updated, ok := directory.Get("slack", "user-1")
	if !ok {
		t.Fatal("expected updated contact to exist")
	}
	if updated.DisplayName != "Alice" || updated.Username != "alice" {
		t.Fatalf("expected missing fields to retain old values, got %+v", updated)
	}
	if !updated.LastSeen.Equal(first.LastSeen) && updated.LastSeen.Before(first.LastSeen) {
		t.Fatalf("expected last seen to stay same or move forward, before=%v after=%v", first.LastSeen, updated.LastSeen)
	}

	directory.AddOrUpdate(ContactInfo{
		UserID:      "user-2",
		Channel:     "discord",
		DisplayName: "Bob",
		Username:    "builder",
	})
	if got := directory.Count(); got != 2 {
		t.Fatalf("expected 2 contacts, got %d", got)
	}
	if contacts := directory.List("slack"); len(contacts) != 1 {
		t.Fatalf("expected 1 slack contact, got %d", len(contacts))
	}
	if results := directory.Search("ali"); len(results) != 1 || results[0].UserID != "user-1" {
		t.Fatalf("unexpected search results: %+v", results)
	}

	wrapped := directory.Wrap(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		return sessionID, "ok", nil
	})
	sessionID, response, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"channel":  "telegram",
		"user_id":  "user-3",
		"username": "charlie",
	})
	if err != nil {
		t.Fatalf("contact wrapper returned error: %v", err)
	}
	if sessionID != "session-1" || response != "ok" {
		t.Fatalf("unexpected contact wrapper result: session=%q response=%q", sessionID, response)
	}
	if _, ok := directory.Get("telegram", "user-3"); !ok {
		t.Fatal("expected wrapped handler to record telegram contact")
	}

	wrappedStream := directory.WrapStream(func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		return sessionID, onChunk("ok")
	})
	var chunks []string
	sessionID, err = wrappedStream(context.Background(), "session-2", "hello", map[string]string{
		"channel":  "signal",
		"user_id":  "user-4",
		"username": "dora",
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("contact stream wrapper returned error: %v", err)
	}
	if sessionID != "session-2" || len(chunks) != 1 || chunks[0] != "ok" {
		t.Fatalf("unexpected contact stream result: session=%q chunks=%v", sessionID, chunks)
	}
	if _, ok := directory.Get("signal", "user-4"); !ok {
		t.Fatal("expected wrapped stream handler to record signal contact")
	}

	directory.Remove("discord", "user-2")
	if got := directory.Count(); got != 3 {
		t.Fatalf("expected 3 contacts after removal, got %d", got)
	}
}

func TestContactDirectoryCleanupAndEviction(t *testing.T) {
	directory := NewContactDirectory()
	directory.SetMaxEntries(2)

	now := time.Now().UTC()
	directory.contacts["slack:user-1"] = ContactInfo{
		UserID:   "user-1",
		Channel:  "slack",
		AddedAt:  now.Add(-3 * time.Hour),
		LastSeen: now.Add(-3 * time.Hour),
	}
	directory.contacts["slack:user-2"] = ContactInfo{
		UserID:   "user-2",
		Channel:  "slack",
		AddedAt:  now.Add(-2 * time.Hour),
		LastSeen: now.Add(-30 * time.Minute),
	}

	directory.AddOrUpdate(ContactInfo{
		UserID:      "user-3",
		Channel:     "slack",
		DisplayName: "Carol",
	})
	if got := directory.Count(); got != 2 {
		t.Fatalf("expected contact count to respect max entries, got %d", got)
	}
	if _, ok := directory.Get("slack", "user-1"); ok {
		t.Fatal("expected oldest contact to be evicted when max entries is exceeded")
	}
	if _, ok := directory.Get("slack", "user-2"); !ok {
		t.Fatal("expected newer contact to remain after eviction")
	}
	if _, ok := directory.Get("slack", "user-3"); !ok {
		t.Fatal("expected newest contact to remain after eviction")
	}

	directory.contacts["discord:user-4"] = ContactInfo{
		UserID:   "user-4",
		Channel:  "discord",
		AddedAt:  now.Add(-48 * time.Hour),
		LastSeen: now.Add(-48 * time.Hour),
	}
	if removed := directory.CleanupStale(time.Hour); removed != 1 {
		t.Fatalf("expected one stale contact to be removed, got %d", removed)
	}
	if _, ok := directory.Get("discord", "user-4"); ok {
		t.Fatal("expected stale contact to be removed by cleanup")
	}

	directory.contacts["telegram:user-5"] = ContactInfo{
		UserID:   "user-5",
		Channel:  "telegram",
		AddedAt:  now.Add(-72 * time.Hour),
		LastSeen: now.Add(-72 * time.Hour),
	}
	directory.SetStaleAfter(time.Hour)
	if _, ok := directory.Get("telegram", "user-5"); ok {
		t.Fatal("expected SetStaleAfter to enforce stale cleanup immediately")
	}
}
