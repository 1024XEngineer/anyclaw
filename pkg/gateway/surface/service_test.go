package surface

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildChannelRawRequest(t *testing.T) {
	meta := map[string]string{
		"user_id":      "u-1",
		"username":     "Ada",
		"thread_id":    "thread-1",
		"group_id":     "group-1",
		"is_group":     "true",
		"agent_name":   "assistant-a",
		"reply_target": "reply-1",
	}

	raw := Service{}.BuildChannelRawRequest(ChannelInput{
		Source:    "telegram",
		SessionID: "session-1",
		Message:   "hello",
		Meta:      meta,
	})

	if raw.SourceType != "channel" || raw.EntryPoint != "channel" {
		t.Fatalf("unexpected source: %s/%s", raw.SourceType, raw.EntryPoint)
	}
	if raw.ChannelID != "telegram" {
		t.Fatalf("unexpected channel id: %q", raw.ChannelID)
	}
	if raw.ActorUserID != "u-1" || raw.ActorDisplayName != "Ada" {
		t.Fatalf("unexpected actor: %q/%q", raw.ActorUserID, raw.ActorDisplayName)
	}
	if !raw.IsGroup || raw.GroupID != "group-1" || raw.ThreadID != "thread-1" {
		t.Fatalf("unexpected group/thread facts: group=%v group_id=%q thread=%q", raw.IsGroup, raw.GroupID, raw.ThreadID)
	}
	if raw.RequestedAgentName != "assistant-a" {
		t.Fatalf("unexpected requested agent: %q", raw.RequestedAgentName)
	}
	if raw.DeliveryReplyTarget != "reply-1" {
		t.Fatalf("unexpected reply target: %q", raw.DeliveryReplyTarget)
	}

	raw.Metadata["user_id"] = "changed"
	if meta["user_id"] != "u-1" {
		t.Fatal("expected metadata to be cloned")
	}
}

func TestBuildSignedWebhookRawRequest(t *testing.T) {
	raw := Service{}.BuildSignedWebhookRawRequest(SignedWebhookInput{
		SessionID: "session-1",
		Title:     "Webhook title",
		Message:   "hello from webhook",
		Meta: map[string]string{
			"source": "test",
		},
	})

	if raw.SourceType != "webhook" || raw.EntryPoint != "webhook" || raw.ChannelID != "webhook" {
		t.Fatalf("unexpected webhook source: %s/%s/%s", raw.SourceType, raw.EntryPoint, raw.ChannelID)
	}
	if raw.SessionID != "session-1" || raw.RequestedSessionID != "session-1" {
		t.Fatalf("unexpected session: %q/%q", raw.SessionID, raw.RequestedSessionID)
	}
	if raw.TitleHint != "Webhook title" || raw.Metadata["title_hint"] != "Webhook title" {
		t.Fatalf("unexpected title hint: %q/%q", raw.TitleHint, raw.Metadata["title_hint"])
	}
	if raw.Message != "hello from webhook" {
		t.Fatalf("unexpected message: %q", raw.Message)
	}
}

func TestWriteUsesSurfaceContract(t *testing.T) {
	recorder := httptest.NewRecorder()

	err := Service{}.Write(recorder, WriteOutput{
		StatusCode: http.StatusAccepted,
		Payload: map[string]string{
			"status": "queued",
		},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type, got %q", recorder.Header().Get("Content-Type"))
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["status"] != "queued" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestWriteErrorUsesSurfaceContract(t *testing.T) {
	recorder := httptest.NewRecorder()

	err := Service{}.WriteError(recorder, http.StatusForbidden, "forbidden")
	if err != nil {
		t.Fatalf("WriteError returned error: %v", err)
	}

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["error"] != "forbidden" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
