package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyWebhookSignatureRejectsMissingSignature(t *testing.T) {
	ext := NewZaloExtension(Config{WebhookToken: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/zalo?timestamp=1710000000", nil)

	if ext.verifyWebhookSignature(map[string]any{"event": "user_send_msg"}, req) {
		t.Fatal("expected missing signature to be rejected")
	}
}

func TestVerifyWebhookSignatureAcceptsValidSignature(t *testing.T) {
	token := "secret"
	timestamp := "1710000000"
	event := map[string]any{"event": "user_send_msg"}
	signature := zaloTestSignature(token, timestamp, event)
	ext := NewZaloExtension(Config{WebhookToken: token})
	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/zalo?timestamp=%s&sig=%s", timestamp, signature),
		nil,
	)

	if !ext.verifyWebhookSignature(event, req) {
		t.Fatal("expected valid signature to be accepted")
	}
}

func TestHandleWebhookRejectsUnsignedPostWhenTokenConfigured(t *testing.T) {
	ext := NewZaloExtension(Config{WebhookToken: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/zalo?timestamp=1710000000", strings.NewReader(`{"event":"user_send_msg"}`))
	rec := httptest.NewRecorder()

	ext.handleWebhook(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected unsigned POST to be forbidden, got %d", rec.Code)
	}
}

func zaloTestSignature(token, timestamp string, event map[string]any) string {
	body, _ := json.Marshal(event)
	sum := sha256.Sum256([]byte(timestamp + token + string(body)))
	return fmt.Sprintf("%x", sum)
}
