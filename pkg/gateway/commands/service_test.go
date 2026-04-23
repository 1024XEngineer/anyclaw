package commands

import "testing"

func TestDispatchChatSendAsIngress(t *testing.T) {
	svc := Service{}

	dispatch, err := svc.Dispatch(NewChatSendCommandRequest(ChatSendRequest{
		Message: "hello",
	}))
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if dispatch.Kind != "ingress" {
		t.Fatalf("expected ingress kind, got %q", dispatch.Kind)
	}
	if dispatch.Target != "ingress" {
		t.Fatalf("expected ingress target, got %q", dispatch.Target)
	}
	if dispatch.Action != "chat.send" {
		t.Fatalf("expected chat.send action, got %q", dispatch.Action)
	}
}

func TestDispatchV2TaskCreateAsMutate(t *testing.T) {
	svc := Service{}

	dispatch, err := svc.Dispatch(NewV2TaskCreateCommandRequest(V2TaskCreateRequest{
		Input: "run task",
	}))
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if dispatch.Kind != "mutate" {
		t.Fatalf("expected mutate kind, got %q", dispatch.Kind)
	}
	if dispatch.Target != "tasks" {
		t.Fatalf("expected tasks target, got %q", dispatch.Target)
	}
	if dispatch.Action != "create" {
		t.Fatalf("expected create action, got %q", dispatch.Action)
	}
}

func TestDispatchSessionsListAsQuery(t *testing.T) {
	svc := Service{}

	dispatch, err := svc.Dispatch(Request{Method: "sessions.list"})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if dispatch.Kind != "query" {
		t.Fatalf("expected query kind, got %q", dispatch.Kind)
	}
	if dispatch.Target != "sessions" {
		t.Fatalf("expected sessions target, got %q", dispatch.Target)
	}
	if dispatch.Action != "list" {
		t.Fatalf("expected list action, got %q", dispatch.Action)
	}
}

func TestCompleteClonesPayload(t *testing.T) {
	svc := Service{}
	payload := map[string]string{"status": "ok"}

	result := svc.Complete(Request{RequestID: "req_1"}, payload, "completed")
	payload["status"] = "changed"

	if result.RequestID != "req_1" {
		t.Fatalf("expected request id req_1, got %q", result.RequestID)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed status, got %q", result.Status)
	}
	if result.Payload["status"] != "ok" {
		t.Fatalf("expected cloned payload, got %q", result.Payload["status"])
	}
}
