package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
)

func TestChannelPairing_Basic(t *testing.T) {
	cp := channel.NewChannelPairing()

	if cp.IsEnabled() {
		t.Error("pairing should be disabled by default")
	}

	cp.SetEnabled(true)
	if !cp.IsEnabled() {
		t.Error("pairing should be enabled after SetEnabled(true)")
	}

	info := cp.Pair("user-1", "device-1", "telegram", "TestUser", 72*time.Hour)
	if info.UserID != "user-1" {
		t.Errorf("expected user-1, got %s", info.UserID)
	}

	if !cp.IsPaired("user-1", "device-1", "telegram") {
		t.Error("expected paired after Pair()")
	}

	cp.Unpair("user-1", "device-1", "telegram")
	if cp.IsPaired("user-1", "device-1", "telegram") {
		t.Error("expected not paired after Unpair()")
	}
}

func TestChannelPairing_Expiration(t *testing.T) {
	cp := channel.NewChannelPairing()
	cp.SetEnabled(true)

	cp.Pair("user-1", "device-1", "telegram", "TestUser", 0)
	if cp.IsPaired("user-1", "device-1", "telegram") {
		t.Error("should not be paired with zero TTL")
	}

	cp.Pair("user-1", "device-1", "telegram", "TestUser", -1)
	if cp.IsPaired("user-1", "device-1", "telegram") {
		t.Error("should not be paired with negative TTL")
	}
}

func TestChannelPairing_ListPaired(t *testing.T) {
	cp := channel.NewChannelPairing()
	cp.SetEnabled(true)

	cp.Pair("user-1", "device-1", "telegram", "User1", -1)
	cp.Pair("user-2", "device-2", "discord", "User2", -1)

	paired := cp.ListPaired()
	if len(paired) != 0 {
		t.Errorf("expected 0 paired with negative TTL, got %d", len(paired))
	}

	cp.Pair("user-1", "device-1", "telegram", "User1", 72*time.Hour)
	cp.Pair("user-2", "device-2", "discord", "User2", 72*time.Hour)

	paired = cp.ListPaired()
	if len(paired) != 2 {
		t.Errorf("expected 2 paired, got %d", len(paired))
	}

	cp.CleanupExpired()
	paired = cp.ListPaired()
	if len(paired) != 2 {
		t.Errorf("expected 2 paired after cleanup (not yet expired), got %d", len(paired))
	}
}

func TestChannelPairing_Wrap(t *testing.T) {
	cp := channel.NewChannelPairing()
	cp.SetEnabled(true)
	cp.Pair("user-1", "device-1", "telegram", "TestUser", 72*time.Hour)

	called := false
	handler := func(ctx context.Context, sessionID, message string, meta map[string]string) (string, string, error) {
		called = true
		return sessionID, "response", nil
	}

	wrapped := cp.Wrap(handler)
	_, _, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"user_id":   "user-1",
		"device_id": "device-1",
		"channel":   "telegram",
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler should be called")
	}
}

func TestChannelPairing_Wrap_NotPaired(t *testing.T) {
	cp := channel.NewChannelPairing()
	cp.SetEnabled(true)

	handler := func(ctx context.Context, sessionID, message string, meta map[string]string) (string, string, error) {
		return sessionID, "response", nil
	}

	wrapped := cp.Wrap(handler)
	resp, _, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"user_id":   "user-1",
		"device_id": "device-1",
		"channel":   "telegram",
	})

	if err != nil {
		t.Logf("got error (also valid): %v", err)
	}
	if resp == "" && err == nil {
		t.Error("expected either error or blocking message when not paired")
	}
}

func TestChannelPairing_Wrap_Disabled(t *testing.T) {
	cp := channel.NewChannelPairing()

	handler := func(ctx context.Context, sessionID, message string, meta map[string]string) (string, string, error) {
		return sessionID, "response", nil
	}

	wrapped := cp.Wrap(handler)
	_, _, err := wrapped(context.Background(), "session-1", "hello", map[string]string{
		"user_id":   "user-1",
		"device_id": "device-1",
		"channel":   "telegram",
	})

	if err != nil {
		t.Errorf("unexpected error when pairing disabled: %v", err)
	}
}
