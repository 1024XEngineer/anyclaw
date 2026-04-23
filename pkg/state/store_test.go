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

func TestSessionGroupFieldsRoundTripAcrossReload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	session := &Session{
		ID:           "sess-group",
		Title:        "Group Session",
		Agent:        "MainAgent",
		Participants: []string{"MainAgent", "Reviewer"},
		Org:          "org-1",
		Project:      "project-1",
		Workspace:    "workspace-1",
		ExecutionBinding: SessionExecutionBinding{
			Agent:     "MainAgent",
			Org:       "org-1",
			Project:   "project-1",
			Workspace: "workspace-1",
		},
		GroupKey:     "group-1",
		IsGroup:      true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		LastActiveAt: time.Now().UTC(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore(reload): %v", err)
	}
	stored, ok := reloaded.GetSession("sess-group")
	if !ok || stored == nil {
		t.Fatal("expected session to load after reload")
	}
	if len(stored.Participants) != 2 {
		t.Fatalf("expected participants after reload, got %#v", stored.Participants)
	}
	if stored.GroupKey != "group-1" {
		t.Fatalf("expected group key after reload, got %q", stored.GroupKey)
	}
	if !stored.IsGroup {
		t.Fatal("expected group flag after reload")
	}
}

func TestRebindSessionsUpdatesExecutionBinding(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	session := &Session{
		ID:        "sess-rebind",
		Title:     "Rebind",
		Agent:     "MainAgent",
		Org:       "org-old",
		Project:   "project-old",
		Workspace: "workspace-1",
		ExecutionBinding: SessionExecutionBinding{
			Agent:     "MainAgent",
			Org:       "org-old",
			Project:   "project-old",
			Workspace: "workspace-1",
		},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		LastActiveAt: time.Now().UTC(),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if err := store.RebindSessionsForProject("project-old", "org-new"); err != nil {
		t.Fatalf("RebindSessionsForProject: %v", err)
	}
	stored, ok := store.GetSession("sess-rebind")
	if !ok || stored == nil {
		t.Fatal("expected rebound session after project rebind")
	}
	if org := SessionExecutionBindingValue(stored).Org; org != "org-new" {
		t.Fatalf("expected execution org to update, got %q", org)
	}

	if err := store.RebindSessionsForWorkspace("workspace-1", "project-new", "org-newer"); err != nil {
		t.Fatalf("RebindSessionsForWorkspace: %v", err)
	}
	stored, ok = store.GetSession("sess-rebind")
	if !ok || stored == nil {
		t.Fatal("expected rebound session after workspace rebind")
	}
	binding := SessionExecutionBindingValue(stored)
	if binding.Project != "project-new" || binding.Org != "org-newer" {
		t.Fatalf("expected execution binding to update, got %#v", binding)
	}
}
