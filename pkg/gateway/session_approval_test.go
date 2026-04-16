package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
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
	if !errors.Is(err, ErrTaskWaitingApproval) {
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
	approvals, ok := payload["approvals"].([]*Approval)
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
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-2",
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
	if !errors.Is(err, ErrTaskWaitingApproval) {
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
	if len(llmStub.messages) < 3 {
		t.Fatalf("expected LLM to be called before approval, after resume, and after tool execution, got %d batches", len(llmStub.messages))
	}
}

func TestSessionToolApprovalReusesApprovedDesktopOpenAcrossMessageChanges(t *testing.T) {
	server, session, _, store := newSessionApprovalTestServer(t, nil)

	args := map[string]any{
		"target": "https://www.douyin.com",
		"kind":   "url",
	}

	err := server.requireSessionToolApproval(session, "", "帮我打开浏览器访问抖音", "api", "desktop_open", args)
	if !errors.Is(err, ErrTaskWaitingApproval) {
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

func newSessionApprovalTestServer(t *testing.T, responses []*llm.Response) (*Server, *Session, *stubTaskLLM, *Store) {
	t.Helper()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.UpsertOrg(&Org{ID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertOrg: %v", err)
	}
	if err := store.UpsertProject(&Project{ID: "project-1", OrgID: "org-1", Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	workspacePath := t.TempDir()
	if err := store.UpsertWorkspace(&Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace", Path: workspacePath}); err != nil {
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
	app := &appRuntime.App{
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
	pool := NewRuntimePool("ignored", store, 4, time.Hour)
	now := time.Now().UTC()
	pool.runtimes[runtimeKey("assistant", "org-1", "project-1", "workspace-1")] = &runtimeEntry{
		app:        app,
		createdAt:  now,
		lastUsedAt: now,
	}
	sessions := NewSessionManager(store, ag)
	session, err := sessions.CreateWithOptions(SessionCreateOptions{
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
		bus:         NewBus(),
		runtimePool: pool,
		approvals:   newApprovalManager(store),
		app: &appRuntime.App{
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
	if !errors.Is(err, ErrTaskWaitingApproval) {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}

	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 || approvals[0].ToolName != "desktop_open" {
		t.Fatalf("expected one desktop_open approval, got %#v", approvals)
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
