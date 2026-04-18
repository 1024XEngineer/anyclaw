package gateway

import (
	"errors"
	"testing"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestSessionExecutionHelpersPreferExecutionBinding(t *testing.T) {
	session := &state.Session{
		Agent:     "legacy-agent",
		Org:       "legacy-org",
		Project:   "legacy-project",
		Workspace: "legacy-workspace",
		ExecutionBinding: state.SessionExecutionBinding{
			Agent:     "binding-agent",
			Org:       "binding-org",
			Project:   "binding-project",
			Workspace: "binding-workspace",
		},
	}

	if got := state.SessionExecutionAgent(session); got != "binding-agent" {
		t.Fatalf("expected binding agent, got %q", got)
	}
	orgID, projectID, workspaceID := state.SessionExecutionHierarchy(session)
	if orgID != "binding-org" || projectID != "binding-project" || workspaceID != "binding-workspace" {
		t.Fatalf("expected binding hierarchy, got org=%q project=%q workspace=%q", orgID, projectID, workspaceID)
	}
	agentName, orgID, projectID, workspaceID := state.SessionExecutionTarget(session)
	if agentName != "binding-agent" || orgID != "binding-org" || projectID != "binding-project" || workspaceID != "binding-workspace" {
		t.Fatalf("expected binding target, got agent=%q org=%q project=%q workspace=%q", agentName, orgID, projectID, workspaceID)
	}
}

func TestRequireSessionToolApprovalUsesExecutionBindingWorkspace(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := state.NewSessionManager(store, nil)
	session, err := manager.CreateWithOptions(state.SessionCreateOptions{
		Title:     "approval session",
		AgentName: "legacy-agent",
		Org:       "legacy-org",
		Project:   "legacy-project",
		Workspace: "legacy-workspace",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	storedSession, ok := store.GetSession(session.ID)
	if !ok {
		t.Fatalf("expected stored session")
	}
	storedSession.ExecutionBinding = state.SessionExecutionBinding{
		Agent:     "binding-agent",
		Org:       "binding-org",
		Project:   "binding-project",
		Workspace: "binding-workspace",
	}
	if err := store.SaveSession(storedSession); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	session, _ = manager.Get(session.ID)
	server := &Server{
		store:     store,
		sessions:  manager,
		approvals: state.NewApprovalManager(store),
	}

	err = server.requireSessionToolApproval(session, session.Title, "run dangerous command", "api", "run_command", map[string]any{"command": "rm -rf /tmp"})
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected ErrTaskWaitingApproval, got %v", err)
	}

	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected one approval, got %d", len(approvals))
	}
	if got, _ := approvals[0].Payload["workspace"].(string); got != "binding-workspace" {
		t.Fatalf("expected approval workspace from execution binding, got %q", got)
	}
}

func TestRecordSessionToolActivitiesUsesExecutionBinding(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	server := &Server{store: store}
	session := &state.Session{
		ID:        "session-1",
		Agent:     "legacy-agent",
		Workspace: "legacy-workspace",
		ExecutionBinding: state.SessionExecutionBinding{
			Agent:     "binding-agent",
			Workspace: "binding-workspace",
		},
	}

	server.recordSessionToolActivities(session, []agent.ToolActivity{{
		ToolName: "desktop_open",
		Args:     map[string]any{"target": "https://example.com"},
		Result:   "ok",
	}})

	activities := store.ListToolActivities(0, session.ID)
	if len(activities) != 1 {
		t.Fatalf("expected one tool activity, got %d", len(activities))
	}
	if activities[0].Agent != "binding-agent" {
		t.Fatalf("expected binding agent in activity, got %q", activities[0].Agent)
	}
	if activities[0].Workspace != "binding-workspace" {
		t.Fatalf("expected binding workspace in activity, got %q", activities[0].Workspace)
	}
}

func TestSessionCreatedEventPayloadUsesExecutionBinding(t *testing.T) {
	payload := sessionCreatedEventPayload(&state.Session{
		Title:     "main",
		Org:       "legacy-org",
		Project:   "legacy-project",
		Workspace: "legacy-workspace",
		ExecutionBinding: state.SessionExecutionBinding{
			Org:       "binding-org",
			Project:   "binding-project",
			Workspace: "binding-workspace",
		},
	})

	if payload["title"] != "main" {
		t.Fatalf("expected title to stay on session payload, got %#v", payload["title"])
	}
	if payload["org"] != "binding-org" || payload["project"] != "binding-project" || payload["workspace"] != "binding-workspace" {
		t.Fatalf("expected binding hierarchy in payload, got %#v", payload)
	}
}

func TestSessionToolApprovalReuseMatchesExecutionBindingWorkspace(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := state.NewSessionManager(store, nil)
	session, err := manager.CreateWithOptions(state.SessionCreateOptions{
		Title:     "approval session",
		AgentName: "legacy-agent",
		Org:       "legacy-org",
		Project:   "legacy-project",
		Workspace: "legacy-workspace",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}

	storedSession, ok := store.GetSession(session.ID)
	if !ok {
		t.Fatalf("expected stored session")
	}
	storedSession.ExecutionBinding.Workspace = "binding-workspace"
	if err := store.SaveSession(storedSession); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	session, _ = manager.Get(session.ID)
	server := &Server{
		store:     store,
		sessions:  manager,
		approvals: state.NewApprovalManager(store),
	}

	err = server.requireSessionToolApproval(session, session.Title, "run dangerous command", "api", "run_command", map[string]any{"command": "echo hi"})
	if !errors.Is(err, taskrunner.ErrTaskWaitingApproval) {
		t.Fatalf("expected first call to wait approval, got %v", err)
	}
	approvals := store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected one approval, got %d", len(approvals))
	}
	if _, err := server.approvals.Resolve(approvals[0].ID, true, "tester", ""); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err = server.requireSessionToolApproval(session, session.Title, "run dangerous command", "api", "run_command", map[string]any{"command": "echo hi"})
	if err != nil {
		t.Fatalf("expected approved signature to be reused, got %v", err)
	}
	approvals = store.ListSessionApprovals(session.ID)
	if len(approvals) != 1 {
		t.Fatalf("expected approval reuse without duplicates, got %d", len(approvals))
	}
}
