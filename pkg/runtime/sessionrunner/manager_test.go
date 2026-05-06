package sessionrunner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	appruntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
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

func TestRunDirectDesktopOpenVerifiesVisibleBrowserWindow(t *testing.T) {
	manager, sessions, session, _, events := newRunManagerTest(t)
	targetURL := "https://www.qiniu.com/"
	toolsRegistry := tools.NewRegistry()
	toolCalls := make([]string, 0, 3)
	windowSnapshots := []string{
		marshalWindowSnapshots(t, []desktopWindowSnapshot{{Title: "Visual Studio Code", ProcessName: "Code", Handle: 11, IsFocused: true}}),
		marshalWindowSnapshots(t, []desktopWindowSnapshot{{Title: "Visual Studio Code", ProcessName: "Code", Handle: 11}, {Title: "Qiniu - Chrome", ProcessName: "chrome", Handle: 99, IsFocused: true}}),
	}
	listIndex := 0
	toolsRegistry.RegisterTool("desktop_open", "Open desktop target", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		toolCalls = append(toolCalls, "desktop_open")
		if got, _ := input["target"].(string); got != targetURL {
			t.Fatalf("expected desktop_open target %q, got %#v", targetURL, input)
		}
		if got, _ := input["kind"].(string); got != "url" {
			t.Fatalf("expected desktop_open kind url, got %#v", input)
		}
		return "opened url", nil
	})
	toolsRegistry.RegisterTool("desktop_list_windows", "List desktop windows", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		toolCalls = append(toolCalls, "desktop_list_windows")
		if listIndex >= len(windowSnapshots) {
			return windowSnapshots[len(windowSnapshots)-1], nil
		}
		result := windowSnapshots[listIndex]
		listIndex++
		return result, nil
	})
	if err := appendApprovedToolApproval(manager.store, session, "desktop_open", map[string]any{"target": targetURL, "kind": "url"}); err != nil {
		t.Fatalf("appendApprovedToolApproval: %v", err)
	}
	runtime := &appruntime.MainRuntime{
		Config: &config.Config{Agent: config.AgentConfig{RequireConfirmationForDangerous: true}},
		Tools:  toolsRegistry,
	}
	manager.runtimes = testRuntimeProvider{runtime: runtime}

	result, err := manager.Run(context.Background(), RunRequest{
		SessionID: session.ID,
		Message:   "打开 https://www.qiniu.com/",
		Options: RunOptions{
			Source: "api",
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil {
		t.Fatal("expected run result")
	}
	wantResponse := "Opened https://www.qiniu.com/ in the desktop browser."
	if result.Response != wantResponse {
		t.Fatalf("expected response %q, got %q", wantResponse, result.Response)
	}
	updated, ok := sessions.Get(session.ID)
	if !ok {
		t.Fatalf("expected session %s to exist", session.ID)
	}
	if len(updated.Messages) != 2 {
		t.Fatalf("expected 2 session messages, got %#v", updated.Messages)
	}
	if updated.Messages[1].Role != "assistant" || updated.Messages[1].Content != wantResponse {
		t.Fatalf("unexpected assistant message %#v", updated.Messages[1])
	}
	activities := manager.store.ListToolActivities(10, session.ID)
	if len(activities) != 2 {
		t.Fatalf("expected 2 tool activities, got %#v", activities)
	}
	if activities[0].ToolName != "desktop_open" || activities[0].Result != "opened url" {
		t.Fatalf("unexpected open activity %#v", activities[0])
	}
	if activities[1].ToolName != "desktop_wait_window" {
		t.Fatalf("expected verification activity, got %#v", activities[1])
	}
	if activities[1].Error != "" {
		t.Fatalf("expected verification success, got %#v", activities[1])
	}
	if activities[1].Result == "" || activities[1].Args["strategy"] != "desktop_wait_window_or_list_windows" {
		t.Fatalf("unexpected verification activity %#v", activities[1])
	}
	if len(toolCalls) < 3 || toolCalls[0] != "desktop_list_windows" || toolCalls[1] != "desktop_open" || toolCalls[2] != "desktop_list_windows" {
		t.Fatalf("unexpected tool call order %#v", toolCalls)
	}
	if !hasEvent(events.events, "chat.completed") {
		t.Fatal("expected chat.completed event")
	}
}

func TestRunDirectDesktopOpenDoesNotClaimSuccessWithoutVerification(t *testing.T) {
	manager, sessions, session, _, events := newRunManagerTest(t)
	targetURL := "https://www.qiniu.com/"
	toolsRegistry := tools.NewRegistry()
	windowPayload := marshalWindowSnapshots(t, []desktopWindowSnapshot{{Title: "Visual Studio Code", ProcessName: "Code", Handle: 11, IsFocused: true}})
	listCalls := 0
	toolsRegistry.RegisterTool("desktop_open", "Open desktop target", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		if got, _ := input["target"].(string); got != targetURL {
			t.Fatalf("expected desktop_open target %q, got %#v", targetURL, input)
		}
		return "opened url", nil
	})
	toolsRegistry.RegisterTool("desktop_list_windows", "List desktop windows", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		listCalls++
		return windowPayload, nil
	})
	if err := appendApprovedToolApproval(manager.store, session, "desktop_open", map[string]any{"target": targetURL, "kind": "url"}); err != nil {
		t.Fatalf("appendApprovedToolApproval: %v", err)
	}
	runtime := &appruntime.MainRuntime{
		Config: &config.Config{Agent: config.AgentConfig{RequireConfirmationForDangerous: true}},
		Tools:  toolsRegistry,
	}
	manager.runtimes = testRuntimeProvider{runtime: runtime}

	result, err := manager.Run(context.Background(), RunRequest{
		SessionID: session.ID,
		Message:   "open https://www.qiniu.com/",
		Options: RunOptions{
			Source: "api",
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantResponse := "Attempted to open https://www.qiniu.com/, but could not verify that a desktop browser window appeared."
	if result == nil || result.Response != wantResponse {
		t.Fatalf("expected response %q, got %#v", wantResponse, result)
	}
	updated, ok := sessions.Get(session.ID)
	if !ok {
		t.Fatalf("expected session %s to exist", session.ID)
	}
	if len(updated.Messages) != 2 || updated.Messages[1].Content != wantResponse {
		t.Fatalf("unexpected session messages %#v", updated.Messages)
	}
	activities := manager.store.ListToolActivities(10, session.ID)
	if len(activities) != 2 {
		t.Fatalf("expected 2 tool activities, got %#v", activities)
	}
	if activities[1].ToolName != "desktop_wait_window" {
		t.Fatalf("expected verification activity, got %#v", activities[1])
	}
	if activities[1].Error == "" {
		t.Fatalf("expected verification failure to be recorded, got %#v", activities[1])
	}
	if listCalls < 2 {
		t.Fatalf("expected repeated window verification polls, got %d", listCalls)
	}
	if !hasEvent(events.events, "chat.completed") {
		t.Fatal("expected chat.completed event")
	}
}

func TestRunDirectDesktopOpenAllowsExistingBrowserWindowReuse(t *testing.T) {
	manager, sessions, session, _, _ := newRunManagerTest(t)
	targetURL := "https://www.qiniu.com/"
	toolsRegistry := tools.NewRegistry()
	windowPayload := marshalWindowSnapshots(t, []desktopWindowSnapshot{
		{Title: "Qiniu - Chrome", ProcessName: "chrome", Handle: 99, IsFocused: true},
		{Title: "Visual Studio Code", ProcessName: "Code", Handle: 11},
	})
	toolsRegistry.RegisterTool("desktop_open", "Open desktop target", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		if got, _ := input["target"].(string); got != targetURL {
			t.Fatalf("expected desktop_open target %q, got %#v", targetURL, input)
		}
		return "opened url", nil
	})
	toolsRegistry.RegisterTool("desktop_list_windows", "List desktop windows", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return windowPayload, nil
	})
	if err := appendApprovedToolApproval(manager.store, session, "desktop_open", map[string]any{"target": targetURL, "kind": "url"}); err != nil {
		t.Fatalf("appendApprovedToolApproval: %v", err)
	}
	runtime := &appruntime.MainRuntime{
		Config: &config.Config{Agent: config.AgentConfig{RequireConfirmationForDangerous: true}},
		Tools:  toolsRegistry,
	}
	manager.runtimes = testRuntimeProvider{runtime: runtime}

	result, err := manager.Run(context.Background(), RunRequest{
		SessionID: session.ID,
		Message:   "open https://www.qiniu.com/",
		Options: RunOptions{
			Source: "api",
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantResponse := "Opened https://www.qiniu.com/ in the desktop browser."
	if result == nil || result.Response != wantResponse {
		t.Fatalf("expected response %q, got %#v", wantResponse, result)
	}
	updated, ok := sessions.Get(session.ID)
	if !ok {
		t.Fatalf("expected session %s to exist", session.ID)
	}
	if len(updated.Messages) != 2 || updated.Messages[1].Content != wantResponse {
		t.Fatalf("unexpected session messages %#v", updated.Messages)
	}
	activities := manager.store.ListToolActivities(10, session.ID)
	if len(activities) != 2 || activities[1].Error != "" {
		t.Fatalf("expected successful verification activity, got %#v", activities)
	}
}

func TestRunDirectDesktopOpenIgnoresHardcodedSiteAliases(t *testing.T) {
	manager, _, session, _, _ := newRunManagerTest(t)
	toolsRegistry := tools.NewRegistry()
	openCalled := false
	toolsRegistry.RegisterTool("desktop_open", "Open desktop target", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		openCalled = true
		return "", errors.New("desktop_open should not be called for implicit site aliases")
	})
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	llmClient := &staticSessionLLM{response: "model handled request"}
	runtime := &appruntime.MainRuntime{
		Config: &config.Config{Agent: config.AgentConfig{Name: "main", RequireConfirmationForDangerous: true}},
		Agent: agent.New(agent.Config{
			Name:   "main",
			LLM:    llmClient,
			Memory: mem,
			Skills: skills.NewSkillsManager(""),
			Tools:  toolsRegistry,
		}),
		Memory: mem,
		Tools:  toolsRegistry,
	}
	manager.runtimes = testRuntimeProvider{runtime: runtime}

	result, err := manager.Run(context.Background(), RunRequest{
		SessionID: session.ID,
		Message:   "帮我打开Edge到七牛云",
		Options: RunOptions{
			Source: "api",
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil || result.Response != "model handled request" {
		t.Fatalf("expected model execution result, got %#v", result)
	}
	if llmClient.calls == 0 {
		t.Fatal("expected implicit site request to fall through to normal model execution")
	}
	if openCalled {
		t.Fatal("did not expect direct desktop_open for implicit site alias")
	}
}

type staticSessionLLM struct {
	response string
	calls    int
}

func (l *staticSessionLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	l.calls++
	return &llm.Response{Content: l.response}, nil
}

func (l *staticSessionLLM) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error {
	l.calls++
	if onChunk != nil {
		onChunk(l.response)
	}
	return nil
}

func (l *staticSessionLLM) Name() string {
	return "static-session-llm"
}

func TestRunChannelReusesApprovedDesktopCapabilityForDifferentDesktopTool(t *testing.T) {
	manager, _, session, approvals, _ := newChannelManagerTest(t)
	if err := appendApprovedCapabilityApproval(manager.store, session, "desktop_open", map[string]any{"target": "https://www.qiniu.com/", "kind": "url"}); err != nil {
		t.Fatalf("appendApprovedCapabilityApproval: %v", err)
	}

	manager.execute = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
		execCtx := tools.WithApprovalGrantScope(ctx)
		err := req.ProtocolApprovalHook(execCtx, tools.ToolApprovalCall{
			Name: "desktop_click",
			Args: map[string]any{"x": 10, "y": 20},
		})
		if err != nil {
			return &appruntime.ExecutionResult{}, err
		}
		if !tools.HasHostReviewedCapability(execCtx, tools.HostReviewedCapabilityDesktop) {
			t.Fatal("expected approved desktop capability to grant host-reviewed desktop context")
		}
		return &appruntime.ExecutionResult{Output: "clicked"}, nil
	}

	result, err := manager.RunChannel(context.Background(), ChannelRunRequest{
		Source:    "slack",
		SessionID: session.ID,
		Message:   "click there",
		QueueMode: "fifo",
		Meta:      map[string]string{"user_id": "u-1"},
	})
	if err != nil {
		t.Fatalf("RunChannel: %v", err)
	}
	if result == nil || result.Response != "clicked" {
		t.Fatalf("unexpected result %#v", result)
	}
	if len(approvals.calls) != 0 {
		t.Fatalf("expected approved desktop capability to avoid a new prompt, got %#v", approvals.calls)
	}
}

func TestRunChannelToolCallDesktopApprovalScopeRequiresNewApprovalForDifferentDesktopTool(t *testing.T) {
	manager, _, session, approvals, _ := newChannelManagerTest(t)
	if err := appendApprovedCapabilityApproval(manager.store, session, "desktop_open", map[string]any{"target": "https://www.qiniu.com/", "kind": "url"}); err != nil {
		t.Fatalf("appendApprovedCapabilityApproval: %v", err)
	}
	manager.runtimes = testRuntimeProvider{runtime: &appruntime.MainRuntime{
		Config: &config.Config{
			Agent: config.AgentConfig{
				RequireConfirmationForDangerous: true,
			},
			Security: config.SecurityConfig{
				DesktopApprovalScope: "tool_call",
			},
		},
	}}

	manager.execute = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
		execCtx := tools.WithApprovalGrantScope(ctx)
		err := req.ProtocolApprovalHook(execCtx, tools.ToolApprovalCall{
			Name: "desktop_click",
			Args: map[string]any{"x": 10, "y": 20},
		})
		return &appruntime.ExecutionResult{}, err
	}

	_, err := manager.RunChannel(context.Background(), ChannelRunRequest{
		Source:    "slack",
		SessionID: session.ID,
		Message:   "click there",
		QueueMode: "fifo",
		Meta:      map[string]string{"user_id": "u-1"},
	})
	if !errors.Is(err, ErrTaskWaitingApproval) {
		t.Fatalf("expected pending approval wait, got %v", err)
	}
	if len(approvals.calls) != 1 || approvals.calls[0].toolName != "desktop_click" {
		t.Fatalf("expected a new desktop_click approval request, got %#v", approvals.calls)
	}
}

func TestRunChannelDoesNotReusePendingDesktopCapability(t *testing.T) {
	manager, _, session, approvals, _ := newChannelManagerTest(t)
	if err := appendDesktopCapabilityApproval(manager.store, session, "desktop_open", map[string]any{"target": "https://www.qiniu.com/", "kind": "url"}, "pending", time.Now().UTC()); err != nil {
		t.Fatalf("appendDesktopCapabilityApproval: %v", err)
	}

	manager.execute = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
		execCtx := tools.WithApprovalGrantScope(ctx)
		err := req.ProtocolApprovalHook(execCtx, tools.ToolApprovalCall{
			Name: "desktop_click",
			Args: map[string]any{"x": 10, "y": 20},
		})
		return &appruntime.ExecutionResult{}, err
	}

	_, err := manager.RunChannel(context.Background(), ChannelRunRequest{
		Source:    "slack",
		SessionID: session.ID,
		Message:   "click there",
		QueueMode: "fifo",
		Meta:      map[string]string{"user_id": "u-1"},
	})
	if !errors.Is(err, ErrTaskWaitingApproval) {
		t.Fatalf("expected pending approval wait, got %v", err)
	}
	if len(approvals.calls) != 1 || approvals.calls[0].toolName != "desktop_click" {
		t.Fatalf("expected a new desktop_click approval request, got %#v", approvals.calls)
	}
}

func TestRunChannelDoesNotReuseExpiredDesktopCapability(t *testing.T) {
	manager, _, session, approvals, _ := newChannelManagerTest(t)
	expiredAt := time.Now().UTC().Add(-20 * time.Minute)
	if err := appendDesktopCapabilityApproval(manager.store, session, "desktop_open", map[string]any{"target": "https://www.qiniu.com/", "kind": "url"}, "approved", expiredAt); err != nil {
		t.Fatalf("appendDesktopCapabilityApproval: %v", err)
	}

	manager.execute = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
		execCtx := tools.WithApprovalGrantScope(ctx)
		err := req.ProtocolApprovalHook(execCtx, tools.ToolApprovalCall{
			Name: "desktop_click",
			Args: map[string]any{"x": 10, "y": 20},
		})
		return &appruntime.ExecutionResult{}, err
	}

	_, err := manager.RunChannel(context.Background(), ChannelRunRequest{
		Source:    "slack",
		SessionID: session.ID,
		Message:   "click there",
		QueueMode: "fifo",
		Meta:      map[string]string{"user_id": "u-1"},
	})
	if !errors.Is(err, ErrTaskWaitingApproval) {
		t.Fatalf("expected pending approval wait for expired grant, got %v", err)
	}
	if len(approvals.calls) != 1 || approvals.calls[0].toolName != "desktop_click" {
		t.Fatalf("expected a new desktop_click approval request, got %#v", approvals.calls)
	}
}

func TestRunChannelDoesNotReuseExpiredExactDesktopToolApproval(t *testing.T) {
	manager, _, session, approvals, _ := newChannelManagerTest(t)
	expiredAt := time.Now().UTC().Add(-20 * time.Minute)
	args := map[string]any{"x": 10, "y": 20}
	if err := appendDesktopCapabilityApproval(manager.store, session, "desktop_click", args, "approved", expiredAt); err != nil {
		t.Fatalf("appendDesktopCapabilityApproval: %v", err)
	}

	manager.execute = func(ctx context.Context, runtime *appruntime.MainRuntime, req appruntime.ExecutionRequest) (*appruntime.ExecutionResult, error) {
		execCtx := tools.WithApprovalGrantScope(ctx)
		err := req.ProtocolApprovalHook(execCtx, tools.ToolApprovalCall{
			Name: "desktop_click",
			Args: args,
		})
		return &appruntime.ExecutionResult{}, err
	}

	_, err := manager.RunChannel(context.Background(), ChannelRunRequest{
		Source:    "slack",
		SessionID: session.ID,
		Message:   "click there again",
		QueueMode: "fifo",
		Meta:      map[string]string{"user_id": "u-1"},
	})
	if !errors.Is(err, ErrTaskWaitingApproval) {
		t.Fatalf("expected pending approval wait for expired exact desktop tool grant, got %v", err)
	}
	if len(approvals.calls) != 1 || approvals.calls[0].toolName != "desktop_click" {
		t.Fatalf("expected a new desktop_click approval request, got %#v", approvals.calls)
	}
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

func newRunManagerTest(t *testing.T) (*Manager, *state.SessionManager, *state.Session, *testApprovalRequester, *testEventRecorder) {
	t.Helper()

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessions := state.NewSessionManager(store, nil)
	session, err := sessions.Create("Run test", "main", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	approvals := &testApprovalRequester{}
	events := &testEventRecorder{}
	manager := NewManager(store, sessions, testRuntimeProvider{}, approvals, events)
	return manager, sessions, session, approvals, events
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

func appendApprovedToolApproval(store *state.Store, session *state.Session, toolName string, args map[string]any) error {
	workspaceID := state.SessionExecutionWorkspace(session)
	signaturePayload := map[string]any{
		"tool_name":  toolName,
		"args":       cloneAnyMap(args),
		"session_id": session.ID,
		"workspace":  workspaceID,
	}
	payload := cloneAnyMap(signaturePayload)
	if capability := sessionApprovalCapability(toolName); capability != "" {
		payload["capability"] = capability
		payload["grant_scope"] = "session"
		payload["grant_ttl_minutes"] = 15
	}
	return store.AppendApproval(&state.Approval{
		ID:        "approval-" + toolName,
		SessionID: session.ID,
		ToolName:  toolName,
		Action:    "tool_call",
		Payload:   payload,
		Signature: approvalSignature(toolName, "tool_call", signaturePayload),
		Status:    "approved",
	})
}

func appendApprovedCapabilityApproval(store *state.Store, session *state.Session, toolName string, args map[string]any) error {
	return appendDesktopCapabilityApproval(store, session, toolName, args, "approved", time.Now().UTC())
}

func appendDesktopCapabilityApproval(store *state.Store, session *state.Session, toolName string, args map[string]any, status string, timestamp time.Time) error {
	workspaceID := state.SessionExecutionWorkspace(session)
	signaturePayload := map[string]any{
		"tool_name":  toolName,
		"args":       cloneAnyMap(args),
		"session_id": session.ID,
		"workspace":  workspaceID,
	}
	payload := cloneAnyMap(signaturePayload)
	payload["capability"] = tools.HostReviewedCapabilityDesktop
	payload["grant_scope"] = "session"
	payload["grant_ttl_minutes"] = 15
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	resolvedAt := ""
	if strings.EqualFold(status, "approved") || strings.EqualFold(status, "rejected") {
		resolvedAt = timestamp.Format(time.RFC3339)
	}
	return store.AppendApproval(&state.Approval{
		ID:          "approval-capability-" + toolName,
		SessionID:   session.ID,
		ToolName:    toolName,
		Action:      "tool_call",
		Payload:     payload,
		Signature:   approvalSignature(toolName, "tool_call", signaturePayload),
		Status:      status,
		RequestedAt: timestamp,
		ResolvedAt:  resolvedAt,
	})
}

func marshalWindowSnapshots(t *testing.T, windows []desktopWindowSnapshot) string {
	t.Helper()
	data, err := json.Marshal(windows)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(data)
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
