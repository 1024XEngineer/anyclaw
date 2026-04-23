package sessionrunner

import (
	"context"
	"errors"
	"testing"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	appruntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type testRuntimeProvider struct {
	runtime *appruntime.MainRuntime
	err     error
}

func (p testRuntimeProvider) GetOrCreate(agentName string, org string, project string, workspaceID string) (*appruntime.MainRuntime, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.runtime, nil
}

type testApprovalRequester struct {
	calls    []approvalCall
	approval *state.Approval
	err      error
}

type approvalCall struct {
	sessionID        string
	toolName         string
	action           string
	payload          map[string]any
	signaturePayload map[string]any
}

func (r *testApprovalRequester) RequestWithSignature(taskID string, sessionID string, stepIndex int, toolName string, action string, payload map[string]any, signaturePayload map[string]any) (*state.Approval, error) {
	r.calls = append(r.calls, approvalCall{
		sessionID:        sessionID,
		toolName:         toolName,
		action:           action,
		payload:          cloneAnyMap(payload),
		signaturePayload: cloneAnyMap(signaturePayload),
	})
	if r.err != nil {
		return nil, r.err
	}
	if r.approval != nil {
		return state.CloneApproval(r.approval), nil
	}
	return &state.Approval{
		ID:        "approval-1",
		SessionID: sessionID,
		ToolName:  toolName,
		Action:    action,
		Status:    "pending",
		Payload:   cloneAnyMap(payload),
	}, nil
}

type testEventRecorder struct {
	events []recordedEvent
}

type recordedEvent struct {
	eventType string
	sessionID string
	payload   map[string]any
}

func (r *testEventRecorder) AppendEvent(eventType string, sessionID string, payload map[string]any) {
	cloned := map[string]any{}
	for k, v := range payload {
		cloned[k] = v
	}
	r.events = append(r.events, recordedEvent{
		eventType: eventType,
		sessionID: sessionID,
		payload:   cloned,
	})
}

func TestRunChannelRequiresApprovalForDangerousTools(t *testing.T) {
	manager, sessions, session, approvals, events := newChannelManagerTest(t)

	manager.execute = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
		if req.AgentApprovalHook == nil {
			t.Fatal("expected AgentApprovalHook to be set for channel execution")
		}
		if req.ProtocolApprovalHook == nil {
			t.Fatal("expected ProtocolApprovalHook to be set for channel execution")
		}
		err := req.AgentApprovalHook(ctx, agent.ToolCall{
			Name: "run_command",
			Args: map[string]any{"command": "rm -rf /tmp/demo"},
		})
		return &appruntime.ExecutionResult{}, err
	}

	result, err := manager.RunChannel(context.Background(), ChannelRunRequest{
		Source:    "slack",
		SessionID: session.ID,
		Message:   "delete that folder",
		QueueMode: "fifo",
		Meta: map[string]string{
			"user_id": "u-1",
		},
	})
	if !errors.Is(err, ErrTaskWaitingApproval) {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}
	if result == nil || result.Session == nil {
		t.Fatal("expected session result when waiting for approval")
	}
	if len(approvals.calls) != 1 {
		t.Fatalf("expected 1 approval request, got %d", len(approvals.calls))
	}
	if approvals.calls[0].toolName != "run_command" || approvals.calls[0].action != "tool_call" {
		t.Fatalf("unexpected approval request: %#v", approvals.calls[0])
	}

	updated, ok := sessions.Get(session.ID)
	if !ok {
		t.Fatalf("expected session %s to exist", session.ID)
	}
	if updated.Presence != "waiting_approval" {
		t.Fatalf("expected waiting_approval presence, got %q", updated.Presence)
	}
	if updated.Typing {
		t.Fatal("expected typing to be false while waiting for approval")
	}
	if updated.QueueDepth != 1 {
		t.Fatalf("expected queue depth 1 while waiting for approval, got %d", updated.QueueDepth)
	}
	if len(updated.Messages) != 1 || updated.Messages[0].Role != "user" || updated.Messages[0].Content != "delete that folder" {
		t.Fatalf("expected pending user message to be preserved, got %#v", updated.Messages)
	}
	if hasEvent(events.events, "chat.failed") {
		t.Fatal("did not expect chat.failed event for approval wait")
	}
	if !hasEvent(events.events, "approval.requested") {
		t.Fatal("expected approval.requested event")
	}
	if !hasEventWithPresence(events.events, "waiting_approval") {
		t.Fatal("expected waiting_approval presence event")
	}
}

func TestRunChannelExecutionErrorsDrainQueuedTurn(t *testing.T) {
	tests := []struct {
		name      string
		streaming bool
	}{
		{name: "execute", streaming: false},
		{name: "stream", streaming: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, sessions, session, _, events := newChannelManagerTest(t)
			runErr := errors.New("runtime exploded")

			if tt.streaming {
				manager.stream = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest, onChunk func(string)) (*appruntime.ExecutionResult, error) {
					if onChunk != nil {
						onChunk("partial")
					}
					return &appruntime.ExecutionResult{Output: "partial"}, runErr
				}
			} else {
				manager.execute = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
					return &appruntime.ExecutionResult{Output: "partial"}, runErr
				}
			}

			result, err := manager.RunChannel(context.Background(), ChannelRunRequest{
				Source:    "discord",
				SessionID: session.ID,
				Message:   "please fail",
				QueueMode: "fifo",
				Streaming: tt.streaming,
				Meta: map[string]string{
					"user_id": "u-2",
				},
			})
			if !errors.Is(err, runErr) {
				t.Fatalf("expected %v, got %v", runErr, err)
			}
			if result == nil {
				t.Fatal("expected result on execution error")
			}
			if result.Response != "partial" {
				t.Fatalf("expected partial response to be preserved, got %q", result.Response)
			}

			updated, ok := sessions.Get(session.ID)
			if !ok {
				t.Fatalf("expected session %s to exist", session.ID)
			}
			if updated.Presence != "idle" {
				t.Fatalf("expected idle presence after failure, got %q", updated.Presence)
			}
			if updated.Typing {
				t.Fatal("expected typing to be false after failure")
			}
			if updated.QueueDepth != 0 {
				t.Fatalf("expected queue depth 0 after failure, got %d", updated.QueueDepth)
			}
			if len(updated.Messages) != 1 || updated.Messages[0].Role != "user" || updated.Messages[0].Content != "please fail" {
				t.Fatalf("expected pending user message to be preserved after failure, got %#v", updated.Messages)
			}
			if !hasEvent(events.events, "chat.failed") {
				t.Fatal("expected chat.failed event on channel execution error")
			}
		})
	}
}

func newChannelManagerTest(t *testing.T) (*Manager, *state.SessionManager, *state.Session, *testApprovalRequester, *testEventRecorder) {
	t.Helper()

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessions := state.NewSessionManager(store, nil)
	session, err := sessions.Create("Channel test", "main", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	approvals := &testApprovalRequester{
		approval: &state.Approval{
			ID:        "approval-1",
			SessionID: session.ID,
			ToolName:  "run_command",
			Action:    "tool_call",
			Status:    "pending",
			Payload:   map[string]any{},
		},
	}
	events := &testEventRecorder{}
	runtime := &appruntime.MainRuntime{
		Config: &config.Config{
			Agent: config.AgentConfig{
				RequireConfirmationForDangerous: true,
			},
		},
	}
	manager := NewManager(store, sessions, testRuntimeProvider{runtime: runtime}, approvals, events)
	return manager, sessions, session, approvals, events
}

func hasEvent(events []recordedEvent, eventType string) bool {
	for _, event := range events {
		if event.eventType == eventType {
			return true
		}
	}
	return false
}

func hasEventWithPresence(events []recordedEvent, presence string) bool {
	for _, event := range events {
		if event.eventType != "session.presence" {
			continue
		}
		if got, _ := event.payload["presence"].(string); got == presence {
			return true
		}
	}
	return false
}
