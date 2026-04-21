package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeleteSessionRemovesSessionApprovals(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	manager := NewSessionManager(store, nil)
	session, err := manager.Create("approval session", "binbin", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.AppendApproval(&Approval{
		ID:          "approval-live",
		SessionID:   session.ID,
		ToolName:    "run_command",
		Action:      "tool_call",
		Signature:   "sig-live",
		Status:      "pending",
		RequestedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendApproval: %v", err)
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if approvals := store.ListApprovals(""); len(approvals) != 0 {
		t.Fatalf("expected session approvals to be removed with session delete, got %#v", approvals)
	}
}

func TestNewStorePrunesOrphanedSessionApprovals(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	payload := persistedState{
		Sessions: []*Session{
			{ID: "sess-live", Title: "Live", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		Approvals: []*Approval{
			{
				ID:          "approval-live",
				SessionID:   "sess-live",
				ToolName:    "run_command",
				Action:      "tool_call",
				Signature:   "sig-live",
				Status:      "pending",
				RequestedAt: time.Now().UTC(),
			},
			{
				ID:          "approval-orphan",
				SessionID:   "sess-missing",
				ToolName:    "run_command",
				Action:      "tool_call",
				Signature:   "sig-orphan",
				Status:      "pending",
				RequestedAt: time.Now().UTC(),
			},
		},
		Events:     []*Event{},
		Tools:      []*ToolActivityRecord{},
		Audit:      []*AuditEvent{},
		Orgs:       []*Org{},
		Projects:   []*Project{},
		Workspaces: []*Workspace{},
		Jobs:       []*Job{},
		Updated:    time.Now().UTC(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(stateFile, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	approvals := store.ListApprovals("")
	if len(approvals) != 1 || approvals[0].ID != "approval-live" {
		t.Fatalf("expected orphaned approval to be pruned on load, got %#v", approvals)
	}
}

func TestNewStoreRepairsPendingSessionMessageFromApprovalPayload(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	payload := persistedState{
		Sessions: []*Session{
			{
				ID:        "sess-live",
				Title:     "Web Chat",
				CreatedAt: time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
				Messages: []SessionMessage{
					{
						ID:        "msg-1",
						Role:      "user",
						Content:   "hello",
						CreatedAt: time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
					},
					{
						ID:        "msg-2",
						Role:      "assistant",
						Content:   "hi",
						CreatedAt: time.Date(2026, 4, 18, 0, 0, 1, 0, time.UTC),
					},
				},
			},
		},
		Approvals: []*Approval{
			{
				ID:        "approval-live",
				SessionID: "sess-live",
				ToolName:  "run_command",
				Action:    "tool_call",
				Payload: map[string]any{
					"message": "create desktop markdown file",
				},
				Signature:   "sig-live",
				Status:      "pending",
				RequestedAt: time.Date(2026, 4, 18, 0, 0, 2, 0, time.UTC),
			},
		},
		Events:     []*Event{},
		Tools:      []*ToolActivityRecord{},
		Audit:      []*AuditEvent{},
		Orgs:       []*Org{},
		Projects:   []*Project{},
		Workspaces: []*Workspace{},
		Jobs:       []*Job{},
		Updated:    time.Now().UTC(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(stateFile, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	session, ok := store.GetSession("sess-live")
	if !ok || session == nil {
		t.Fatal("expected repaired session to load")
	}
	if len(session.Messages) != 3 {
		t.Fatalf("expected pending approval message to be restored, got %#v", session.Messages)
	}
	lastMessage := session.Messages[len(session.Messages)-1]
	if lastMessage.Role != "user" || lastMessage.Content != "create desktop markdown file" {
		t.Fatalf("expected restored user message from approval payload, got %#v", lastMessage)
	}
}

func TestRebindSessionsForWorkspaceUpdatesExecutionBinding(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	session := &Session{
		ID:        "sess-1",
		Title:     "Workspace session",
		Org:       "org-old",
		Project:   "project-old",
		Workspace: "workspace-1",
		ExecutionBinding: SessionExecutionBinding{
			Org:       "org-old",
			Project:   "project-old",
			Workspace: "workspace-1",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if err := store.RebindSessionsForWorkspace("workspace-1", "project-new", "org-new"); err != nil {
		t.Fatalf("RebindSessionsForWorkspace: %v", err)
	}

	updated, ok := store.GetSession("sess-1")
	if !ok || updated == nil {
		t.Fatal("expected session to exist")
	}
	if updated.Org != "org-new" || updated.ExecutionBinding.Org != "org-new" {
		t.Fatalf("expected org to rebind, got session=%q binding=%q", updated.Org, updated.ExecutionBinding.Org)
	}
	if updated.Project != "project-new" || updated.ExecutionBinding.Project != "project-new" {
		t.Fatalf("expected project to rebind, got session=%q binding=%q", updated.Project, updated.ExecutionBinding.Project)
	}
}

func TestRebindWorkspaceIDUpdatesExecutionBinding(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	store.workspaces = append(store.workspaces, &Workspace{
		ID:        "workspace-old",
		ProjectID: "project-1",
		Name:      "Workspace",
		Path:      "/tmp/workspace-old",
	})
	session := &Session{
		ID:        "sess-1",
		Title:     "Workspace session",
		Workspace: "workspace-old",
		ExecutionBinding: SessionExecutionBinding{
			Workspace: "workspace-old",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if err := store.RebindWorkspaceID("workspace-old", "workspace-new"); err != nil {
		t.Fatalf("RebindWorkspaceID: %v", err)
	}

	updated, ok := store.GetSession("sess-1")
	if !ok || updated == nil {
		t.Fatal("expected session to exist")
	}
	if updated.Workspace != "workspace-new" || updated.ExecutionBinding.Workspace != "workspace-new" {
		t.Fatalf("expected workspace to rebind, got session=%q binding=%q", updated.Workspace, updated.ExecutionBinding.Workspace)
	}
}
