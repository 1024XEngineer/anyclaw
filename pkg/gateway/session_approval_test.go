package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/capability/agents"
	"github.com/anyclaw/anyclaw/pkg/capability/models"
	"github.com/anyclaw/anyclaw/pkg/capability/skills"
	"github.com/anyclaw/anyclaw/pkg/capability/tools"
	"github.com/anyclaw/anyclaw/pkg/config"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	taskrunner "github.com/anyclaw/anyclaw/pkg/runtime/taskrunner"
	"github.com/anyclaw/anyclaw/pkg/state"
	"github.com/anyclaw/anyclaw/pkg/state/memory"
)

func TestRunSessionMessageWaitsForSessionToolApproval(t *testing.T) {
	server, session, _, store := newSessionApprovalTestServer(t, []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"echo hi"}`,
					},
				},
			},
		},
	})

	response, updatedSession, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "run dangerous command")
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected ErrTaskWaitingApproval, got response=%q session=%#v err=%v", response, updatedSession, err)
	}
	if updatedSession == nil || updatedSession.ID != session.ID {
		t.Fatalf("expected session to be returned while waiting approval, got %#v", updatedSession)
	}
	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected 1 session approval, got %d", len(approvals))
	}
	if approvals[0].ToolName != "run_command" || approvals[0].Status != "pending" {
		t.Fatalf("unexpected approval payload: %#v", approvals[0])
	}
	if approvals[0].Payload["message"] != "run dangerous command" {
		t.Fatalf("expected approval payload to include original message, got %#v", approvals[0].Payload)
	}
	freshSession, ok := server.sessions.Get(session.ID)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if len(freshSession.Messages) != 1 {
		t.Fatalf("expected pending approval to keep the user message, got %#v", freshSession.Messages)
	}
	if freshSession.Messages[0].Role != "user" || freshSession.Messages[0].Content != "run dangerous command" {
		t.Fatalf("expected stored pending user message, got %#v", freshSession.Messages)
	}
	if freshSession.Presence != "waiting_approval" || freshSession.Typing {
		t.Fatalf("expected session waiting_approval without typing, got presence=%q typing=%v", freshSession.Presence, freshSession.Typing)
	}
}

func TestWSChatSendReturnsWaitingApprovalPayload(t *testing.T) {
	server, session, _, _ := newSessionApprovalTestServer(t, []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"echo hi"}`,
					},
				},
			},
		},
	})

	payload, err := server.wsChatSend(context.Background(), nil, map[string]any{
		"session_id": session.ID,
		"message":    "run dangerous command",
	})
	if err != nil {
		t.Fatalf("wsChatSend: %v", err)
	}
	if payload["status"] != "waiting_approval" {
		t.Fatalf("expected waiting_approval payload, got %#v", payload)
	}
	approvals, ok := payload["approvals"].([]*state.Approval)
	if !ok || len(approvals) != 1 {
		t.Fatalf("expected session approvals in payload, got %#v", payload["approvals"])
	}
}

func TestResumeApprovedSessionApprovalCompletesExchange(t *testing.T) {
	server, session, llmStub, store := newSessionApprovalTestServer(t, []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"echo hi"}`,
					},
				},
			},
		},
		{Content: "done"},
	})

	_, _, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "run dangerous command")
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}
	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected 1 session approval, got %d", len(approvals))
	}
	updatedApproval, err := server.approvals.Resolve(approvals[0].ID, true, "tester", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := server.resumeApprovedSessionApproval(context.Background(), updatedApproval); err != nil {
		t.Fatalf("resumeApprovedSessionApproval: %v", err)
	}
	freshSession, ok := server.sessions.Get(session.ID)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if len(freshSession.Messages) != 2 {
		t.Fatalf("expected completed user/assistant exchange, got %#v", freshSession.Messages)
	}
	if freshSession.Messages[0].Role != "user" || freshSession.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected session messages: %#v", freshSession.Messages)
	}
	if freshSession.Messages[1].Content != "done" {
		t.Fatalf("expected assistant response after approval resume, got %#v", freshSession.Messages[1])
	}
	if freshSession.Presence != "idle" || freshSession.Typing {
		t.Fatalf("expected idle session after resume, got presence=%q typing=%v", freshSession.Presence, freshSession.Typing)
	}
	activities := store.ListToolActivities(10, session.ID)
	if len(activities) != 1 || activities[0].ToolName != "run_command" {
		t.Fatalf("expected resumed tool activity to be recorded, got %#v", activities)
	}
	if len(llmStub.messages) != 2 {
		t.Fatalf("expected LLM to be called once before approval and once after the approved tool ran, got %d batches", len(llmStub.messages))
	}
}

func TestResumeApprovedSessionApprovalKeepsUserMessageWhenAnotherApprovalIsRequired(t *testing.T) {
	server, session, _, store := newSessionApprovalTestServer(t, []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"echo %USERPROFILE%"}`,
					},
				},
			},
		},
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-2",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"dir \"%USERPROFILE%\\\\Desktop\""}`,
					},
				},
			},
		},
	})

	_, _, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "create desktop markdown file")
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected initial approval wait, got %v", err)
	}

	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected 1 initial approval, got %d", len(approvals))
	}

	updatedApproval, err := server.approvals.Resolve(approvals[0].ID, true, "tester", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err = server.resumeApprovedSessionApproval(context.Background(), updatedApproval)
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected a second approval wait after resume, got %v", err)
	}

	freshSession, ok := server.sessions.Get(session.ID)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if len(freshSession.Messages) != 1 {
		t.Fatalf("expected only the stored user message while waiting on the second approval, got %#v", freshSession.Messages)
	}
	if freshSession.Messages[0].Role != "user" || freshSession.Messages[0].Content != "create desktop markdown file" {
		t.Fatalf("expected pending user message to survive the second approval, got %#v", freshSession.Messages)
	}
	if freshSession.Presence != "waiting_approval" || freshSession.Typing {
		t.Fatalf("expected session waiting_approval without typing, got presence=%q typing=%v", freshSession.Presence, freshSession.Typing)
	}

	approvals = store.ListSessionApprovals(session.ID)
	if len(approvals) != 2 {
		t.Fatalf("expected two approvals after the resumed tool call requested another one, got %#v", approvals)
	}
	activities := store.ListToolActivities(10, session.ID)
	if len(activities) != 1 || activities[0].ToolName != "run_command" {
		t.Fatalf("expected the approved tool to execute before the next approval was requested, got %#v", activities)
	}
}

func TestRunSessionMessageFailurePersistsUserMessageAndClearsQueue(t *testing.T) {
	server, session, llmStub, _ := newSessionApprovalTestServer(t, nil)
	llmStub.chatFunc = func(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
		return nil, context.DeadlineExceeded
	}

	response, updatedSession, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "你好")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got response=%q session=%#v err=%v", response, updatedSession, err)
	}
	if updatedSession == nil {
		t.Fatal("expected failed run to return session state")
	}
	if updatedSession.QueueDepth != 0 {
		t.Fatalf("expected failed run to clear queue depth, got %d", updatedSession.QueueDepth)
	}
	if updatedSession.Presence != "idle" || updatedSession.Typing {
		t.Fatalf("expected failed run to leave session idle, got presence=%q typing=%v", updatedSession.Presence, updatedSession.Typing)
	}
	if len(updatedSession.Messages) != 1 || updatedSession.Messages[0].Role != "user" || updatedSession.Messages[0].Content != "你好" {
		t.Fatalf("expected failed run to persist the user message, got %#v", updatedSession.Messages)
	}
}

func TestLegacyApprovalResumeTimeoutReleasesRuntimeForNextMessage(t *testing.T) {
	server, session, llmStub, store := newSessionApprovalTestServer(t, []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"echo hi"}`,
					},
				},
			},
		},
		{Content: "recovered"},
	})

	_, _, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "run dangerous command")
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected initial approval wait, got %v", err)
	}

	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(approvals))
	}
	delete(approvals[0].Payload, "resume_state")
	if err := store.UpdateApproval(approvals[0]); err != nil {
		t.Fatalf("UpdateApproval: %v", err)
	}

	resumeCancelled := make(chan error, 1)
	llmStub.chatFunc = func(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
		select {
		case <-ctx.Done():
			resumeCancelled <- ctx.Err()
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
			return &llm.Response{Content: "unexpected"}, nil
		}
	}

	updatedApproval, err := server.approvals.Resolve(approvals[0].ID, true, "tester", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	svc := server.approvalsService()
	svc.SessionResumeTimeout = 25 * time.Millisecond
	svc.HandleResolved(updatedApproval, true, "")

	select {
	case err := <-resumeCancelled:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected resume timeout cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected legacy approval resume to be cancelled by timeout")
	}

	llmStub.chatFunc = nil

	deadline := time.Now().Add(time.Second)
	for {
		freshSession, ok := server.sessions.Get(session.ID)
		if !ok {
			t.Fatal("expected session to exist")
		}
		if freshSession.QueueDepth == 0 && freshSession.Presence == "idle" && !freshSession.Typing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected timed out resume to settle the session, got %#v", freshSession)
		}
		time.Sleep(10 * time.Millisecond)
	}

	response, updatedSession, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "hello again")
	if err != nil {
		t.Fatalf("expected follow-up run to succeed after timeout cleanup, got response=%q session=%#v err=%v", response, updatedSession, err)
	}
	if response != "recovered" {
		t.Fatalf("expected follow-up run to use the next LLM response, got %q", response)
	}
	if updatedSession == nil || updatedSession.QueueDepth != 0 {
		t.Fatalf("expected follow-up run to complete cleanly, got %#v", updatedSession)
	}
}

func TestSessionToolApprovalReusesApprovedDesktopOpenAcrossMessageChanges(t *testing.T) {
	server, session, _, store := newSessionApprovalTestServer(t, nil)

	args := map[string]any{
		"target": "https://www.douyin.com",
		"kind":   "url",
	}

	err := server.requireSessionToolApproval(session, "", "帮我打开浏览器访问抖音", "api", "desktop_open", args)
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}

	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(approvals))
	}

	if _, err := server.approvals.Resolve(approvals[0].ID, true, "tester", ""); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err = server.requireSessionToolApproval(session, "", "继续", "api", "desktop_open", args)
	if err != nil {
		t.Fatalf("expected approved desktop_open to be reused, got %v", err)
	}

	approvals = store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected approval reuse without duplicates, got %d approvals", len(approvals))
	}
}

func newSessionApprovalTestServer(t *testing.T, responses []*llm.Response) (*Server, *state.Session, *stubTaskLLM, *state.Store) {
	t.Helper()

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.UpsertOrg(&state.Org{ID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertOrg: %v", err)
	}
	if err := store.UpsertProject(&state.Project{ID: "project-1", OrgID: "org-1", Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	workspacePath := t.TempDir()
	if err := store.UpsertWorkspace(&state.Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace", Path: workspacePath}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "run", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "ok", nil
	})
	registry.RegisterTool("desktop_open", "open desktop app", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "opened", nil
	})
	llmStub := &stubTaskLLM{responses: responses}
	ag := agent.New(agent.Config{
		Name:        "assistant",
		Description: "test assistant",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		WorkDir:     t.TempDir(),
	})
	mainRuntime := &appRuntime.App{
		Config: &config.Config{
			Agent: config.AgentConfig{
				Name:                            "assistant",
				RequireConfirmationForDangerous: true,
			},
		},
		Agent:      ag,
		Tools:      registry,
		WorkingDir: workspacePath,
		WorkDir:    t.TempDir(),
	}
	pool := appRuntime.NewRuntimePool("ignored", store, 4, time.Hour)
	pool.Remember("assistant", "org-1", "project-1", "workspace-1", mainRuntime)
	sessions := state.NewSessionManager(store, ag)
	session, err := sessions.CreateWithOptions(state.SessionCreateOptions{
		Title:       "approval session",
		AgentName:   "assistant",
		Org:         "org-1",
		Project:     "project-1",
		Workspace:   "workspace-1",
		SessionMode: "main",
		QueueMode:   "fifo",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	server := &Server{
		store:       store,
		sessions:    sessions,
		bus:         state.NewEventBus(),
		runtimePool: pool,
		approvals:   state.NewApprovalManager(store),
		mainRuntime: &appRuntime.App{
			Config: &config.Config{
				Agent: config.AgentConfig{Name: "assistant"},
			},
			WorkingDir: workspacePath,
		},
	}
	return server, session, llmStub, store
}

func TestResumeApprovedDesktopOpenShortcutCompletesWithoutLLM(t *testing.T) {
	server, session, llmStub, store := newSessionApprovalTestServer(t, nil)

	_, _, err := server.runSessionMessage(context.Background(), session.ID, session.Title, "Open browser https://www.douyin.com/")
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}

	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 || approvals[0].ToolName != "desktop_open" {
		t.Fatalf("expected one desktop_open approval, got %#v", approvals)
	}
	freshSession, ok := server.sessions.Get(session.ID)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if len(freshSession.Messages) != 1 || freshSession.Messages[0].Role != "user" {
		t.Fatalf("expected desktop_open approval wait to persist the user message, got %#v", freshSession.Messages)
	}

	updatedApproval, err := server.approvals.Resolve(approvals[0].ID, true, "tester", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if err := server.resumeApprovedSessionApproval(context.Background(), updatedApproval); err != nil {
		t.Fatalf("resumeApprovedSessionApproval: %v", err)
	}

	freshSession, ok = server.sessions.Get(session.ID)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if len(freshSession.Messages) != 2 {
		t.Fatalf("expected completed user/assistant exchange, got %#v", freshSession.Messages)
	}
	if freshSession.Messages[1].Content == "" || !strings.Contains(freshSession.Messages[1].Content, "https://www.douyin.com/") {
		t.Fatalf("expected assistant to confirm desktop open, got %#v", freshSession.Messages[1])
	}
	if freshSession.Presence != "idle" || freshSession.Typing {
		t.Fatalf("expected idle session after desktop_open shortcut, got presence=%q typing=%v", freshSession.Presence, freshSession.Typing)
	}
	activities := store.ListToolActivities(10, session.ID)
	if len(activities) != 1 || activities[0].ToolName != "desktop_open" {
		t.Fatalf("expected desktop_open activity to be recorded, got %#v", activities)
	}
	if len(llmStub.messages) != 0 {
		t.Fatalf("expected desktop_open shortcut to bypass the LLM, got %d LLM calls", len(llmStub.messages))
	}
}
