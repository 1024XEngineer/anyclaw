package isolation

import (
	"context"
	"testing"

	ctxpkg "github.com/1024XEngineer/anyclaw/pkg/runtime/context/store"
)

func TestNewContextIsolationManager(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	if mgr == nil {
		t.Fatal("expected manager to be created")
	}
	defer mgr.Close()
}

func TestCreateBoundary(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
		Namespace: "test",
	}

	boundary, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if boundary.Scope.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got '%s'", boundary.Scope.AgentID)
	}

	if boundary.Mode != IsolationModeStrict {
		t.Errorf("expected mode 'strict', got '%s'", boundary.Mode)
	}

	if boundary.Visibility != ContextVisibilityPrivate {
		t.Errorf("expected visibility 'private', got '%s'", boundary.Visibility)
	}
}

func TestCreateBoundaryDuplicate(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}

	_, err = mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err == nil {
		t.Fatal("expected error for duplicate boundary")
	}
}

func TestGetBoundary(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	boundary, ok := mgr.GetBoundary("session-1")
	if !ok {
		t.Fatal("expected boundary to exist")
	}

	if boundary.Scope.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got '%s'", boundary.Scope.AgentID)
	}

	_, ok = mgr.GetBoundary("nonexistent")
	if ok {
		t.Fatal("expected boundary to not exist")
	}
}

func TestGetEngine(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	engine, ok := mgr.GetEngine("session-1")
	if !ok {
		t.Fatal("expected engine to exist")
	}

	if engine.Name() != "isolated-session-1" {
		t.Errorf("expected engine name 'isolated-session-1', got '%s'", engine.Name())
	}
}

func TestCreateChildBoundary(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	parentScope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(parentScope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create parent boundary: %v", err)
	}

	childScope := &ContextScope{
		AgentID:   "agent-1",
		TaskID:    "task-1",
		Namespace: "child",
	}

	childBoundary, err := mgr.CreateChildBoundary("session-1", childScope, IsolationModeHybrid, ContextVisibilityScoped)
	if err != nil {
		t.Fatalf("failed to create child boundary: %v", err)
	}

	if childBoundary.Parent == nil {
		t.Fatal("expected child boundary to have parent")
	}

	if childBoundary.Parent.Scope.SessionID != "session-1" {
		t.Errorf("expected parent session ID 'session-1', got '%s'", childBoundary.Parent.Scope.SessionID)
	}

	if childBoundary.Mode != IsolationModeHybrid {
		t.Errorf("expected child mode 'hybrid', got '%s'", childBoundary.Mode)
	}
}

func TestAddDocument(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
		Namespace: "test",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	engine, ok := mgr.GetEngine("session-1")
	if !ok {
		t.Fatal("expected engine to exist")
	}

	doc := ctxpkg.Document{
		ID:      "doc-1",
		Content: "This is a test document about Go programming",
		Metadata: map[string]any{
			"type": "test",
		},
	}

	ctx := context.Background()
	err = engine.AddDocument(ctx, doc)
	if err != nil {
		t.Fatalf("failed to add document: %v", err)
	}

	retrieved, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("failed to get document: %v", err)
	}

	if retrieved.Content != doc.Content {
		t.Errorf("expected content '%s', got '%s'", doc.Content, retrieved.Content)
	}

	if retrieved.Metadata["agent_id"] != "agent-1" {
		t.Errorf("expected metadata agent_id 'agent-1', got '%v'", retrieved.Metadata["agent_id"])
	}
}

func TestSearchDocuments(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	engine, ok := mgr.GetEngine("session-1")
	if !ok {
		t.Fatal("expected engine to exist")
	}

	ctx := context.Background()
	docs := []ctxpkg.Document{
		{ID: "doc-1", Content: "Go programming language is great for building systems"},
		{ID: "doc-2", Content: "Python is good for data science and machine learning"},
		{ID: "doc-3", Content: "Go concurrency model uses goroutines and channels"},
	}

	for _, doc := range docs {
		if err := engine.AddDocument(ctx, doc); err != nil {
			t.Fatalf("failed to add document %s: %v", doc.ID, err)
		}
	}

	results, err := engine.Search(ctx, "Go programming", ctxpkg.SearchOptions{
		TopK:      2,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if len(results) > 0 && results[0].Document.ID != "doc-1" && results[0].Document.ID != "doc-3" {
		t.Logf("first result should be Go-related, got: %s", results[0].Document.ID)
	}
}

func TestDeleteDocument(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	engine, ok := mgr.GetEngine("session-1")
	if !ok {
		t.Fatal("expected engine to exist")
	}

	ctx := context.Background()
	doc := ctxpkg.Document{ID: "doc-1", Content: "test content"}

	if err := engine.AddDocument(ctx, doc); err != nil {
		t.Fatalf("failed to add document: %v", err)
	}

	if err := engine.DeleteDocument(ctx, "doc-1"); err != nil {
		t.Fatalf("failed to delete document: %v", err)
	}

	_, err = engine.GetDocument(ctx, "doc-1")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestContextIsolation(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	engine1, _ := mgr.GetEngine("session-1")
	engine2, _ := mgr.GetEngine("session-2")

	ctx := context.Background()
	doc := ctxpkg.Document{ID: "doc-1", Content: "isolated content"}

	if err := engine1.AddDocument(ctx, doc); err != nil {
		t.Fatalf("failed to add document to engine1: %v", err)
	}

	_, err = engine2.GetDocument(ctx, "doc-1")
	if err == nil {
		t.Fatal("expected document to be isolated, but it was accessible from engine2")
	}
}

func TestSharingPolicy(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	policy := &SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2", "agent-3"},
		Namespace:      "shared",
	}

	if err := mgr.AddSharingPolicy(policy); err != nil {
		t.Fatalf("failed to add policy: %v", err)
	}

	if !mgr.CanShareContext("agent-1", "agent-2", "shared") {
		t.Error("expected sharing to be allowed")
	}

	if !mgr.CanShareContext("agent-1", "agent-3", "shared") {
		t.Error("expected sharing to be allowed")
	}

	if mgr.CanShareContext("agent-2", "agent-1", "shared") {
		t.Error("expected sharing to be denied (one-way)")
	}

	if mgr.CanShareContext("agent-1", "agent-2", "other") {
		t.Error("expected sharing to be denied (wrong namespace)")
	}
}

func TestRemoveSharingPolicy(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	policy := &SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2", "agent-3"},
	}

	if err := mgr.AddSharingPolicy(policy); err != nil {
		t.Fatalf("failed to add policy: %v", err)
	}

	removed := mgr.RemoveSharingPolicy("agent-1", "agent-2")
	if !removed {
		t.Fatal("expected policy to be removed")
	}

	if mgr.CanShareContext("agent-1", "agent-2", "") {
		t.Error("expected sharing to be denied after removal")
	}

	if !mgr.CanShareContext("agent-1", "agent-3", "") {
		t.Error("expected sharing to still be allowed for agent-3")
	}
}

func TestCreateSnapshot(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSnapshots = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	engine, _ := mgr.GetEngine("session-1")
	ctx := context.Background()

	doc := ctxpkg.Document{ID: "doc-1", Content: "snapshot content"}
	if err := engine.AddDocument(ctx, doc); err != nil {
		t.Fatalf("failed to add document: %v", err)
	}

	snapshot, err := mgr.CreateSnapshot("agent-1", "session-1", "test snapshot")
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	if len(snapshot.Documents) != 1 {
		t.Errorf("expected 1 document in snapshot, got %d", len(snapshot.Documents))
	}

	if snapshot.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got '%s'", snapshot.AgentID)
	}

	retrieved, ok := mgr.GetSnapshot(snapshot.ID)
	if !ok {
		t.Fatal("expected snapshot to exist")
	}

	if retrieved.ID != snapshot.ID {
		t.Errorf("expected snapshot ID '%s', got '%s'", snapshot.ID, retrieved.ID)
	}
}

func TestListSnapshots(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSnapshots = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	_, err = mgr.CreateSnapshot("agent-1", "session-1", "snapshot 1")
	if err != nil {
		t.Fatalf("failed to create snapshot 1: %v", err)
	}

	_, err = mgr.CreateSnapshot("agent-1", "session-1", "snapshot 2")
	if err != nil {
		t.Fatalf("failed to create snapshot 2: %v", err)
	}

	snapshots := mgr.ListSnapshots("agent-1")
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}

	allSnapshots := mgr.ListSnapshots("")
	if len(allSnapshots) != 2 {
		t.Errorf("expected 2 total snapshots, got %d", len(allSnapshots))
	}
}

func TestDeleteBoundary(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	if err := mgr.DeleteBoundary("session-1"); err != nil {
		t.Fatalf("failed to delete boundary: %v", err)
	}

	_, ok := mgr.GetBoundary("session-1")
	if ok {
		t.Fatal("expected boundary to be deleted")
	}

	_, ok = mgr.GetEngine("session-1")
	if ok {
		t.Fatal("expected engine to be deleted")
	}
}

func TestDeleteBoundaryWithChildren(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	parentScope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(parentScope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create parent boundary: %v", err)
	}

	childScope := &ContextScope{
		AgentID: "agent-1",
		TaskID:  "task-1",
	}

	_, err = mgr.CreateChildBoundary("session-1", childScope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create child boundary: %v", err)
	}

	if err := mgr.DeleteBoundary("session-1"); err != nil {
		t.Fatalf("failed to delete parent boundary: %v", err)
	}

	_, ok := mgr.GetBoundary("task-1")
	if ok {
		t.Fatal("expected child boundary to be deleted")
	}
}

func TestListBoundaries(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	boundaries := mgr.ListBoundaries()
	if len(boundaries) != 2 {
		t.Errorf("expected 2 boundaries, got %d", len(boundaries))
	}
}

func TestSharedSearch(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	policy := &SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
	}
	if err := mgr.AddSharingPolicy(policy); err != nil {
		t.Fatalf("failed to add policy: %v", err)
	}

	kvEngine1, _ := mgr.GetKVEngine("session-1")
	ctx := context.Background()
	kvEngine1.Set(ctx, "key1", "value1")
	kvEngine1.Set(ctx, "key2", "value2")

	results, err := mgr.SharedSearch("session-2", "", "")
	if err != nil {
		t.Fatalf("shared search failed: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("expected at least 1 shared result, got %d", len(results))
	}
}

func TestContextScopeMiddleware(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)

	scopeID, err := middleware.EnterScope("agent-1", "session-1", "", "test", IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to enter scope: %v", err)
	}

	if scopeID != "session-1" {
		t.Errorf("expected scope ID 'session-1', got '%s'", scopeID)
	}

	if !middleware.IsScoped("agent-1") {
		t.Error("expected agent to be scoped")
	}

	currentScope, err := middleware.GetCurrentScope("agent-1")
	if err != nil {
		t.Fatalf("failed to get current scope: %v", err)
	}

	if currentScope.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got '%s'", currentScope.AgentID)
	}

	engine, err := middleware.GetCurrentEngine("agent-1")
	if err != nil {
		t.Fatalf("failed to get current engine: %v", err)
	}

	if engine == nil {
		t.Fatal("expected engine to be returned")
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit scope: %v", err)
	}

	if middleware.IsScoped("agent-1") {
		t.Error("expected agent to no longer be scoped")
	}
}

func TestContextScopeMiddlewareNested(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)

	_, err := middleware.EnterScope("agent-1", "session-1", "", "test", IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to enter first scope: %v", err)
	}

	_, err = middleware.EnterScope("agent-1", "session-2", "task-1", "test", IsolationModeHybrid, ContextVisibilityScoped)
	if err != nil {
		t.Fatalf("failed to enter nested scope: %v", err)
	}

	stack := middleware.GetScopeStack("agent-1")
	if len(stack) != 2 {
		t.Errorf("expected stack depth 2, got %d", len(stack))
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit nested scope: %v", err)
	}

	if !middleware.IsScoped("agent-1") {
		t.Error("expected agent to still be scoped after exiting nested scope")
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit outer scope: %v", err)
	}

	if middleware.IsScoped("agent-1") {
		t.Error("expected agent to no longer be scoped")
	}
}

func TestContextEnforcer(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)
	enforcer := NewContextEnforcer(middleware)

	enforcer.AddRule("agent-1", &EnforcementRule{
		AgentID:            "agent-1",
		RequiredMode:       IsolationModeStrict,
		RequiredVisibility: ContextVisibilityPrivate,
		MaxNestedDepth:     2,
	})

	_, err := middleware.EnterScope("agent-1", "session-1", "", "test", IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to enter scope: %v", err)
	}

	if err := enforcer.Enforce("agent-1"); err != nil {
		t.Errorf("expected enforcement to pass: %v", err)
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit scope: %v", err)
	}
}

func TestContextEnforcerViolation(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)
	enforcer := NewContextEnforcer(middleware)

	enforcer.AddRule("agent-1", &EnforcementRule{
		AgentID:            "agent-1",
		RequiredMode:       IsolationModeStrict,
		RequiredVisibility: ContextVisibilityPrivate,
	})

	_, err := middleware.EnterScope("agent-1", "session-1", "", "test", IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to enter scope: %v", err)
	}

	err = enforcer.Enforce("agent-1")
	if err == nil {
		t.Fatal("expected enforcement to fail due to mode violation")
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit scope: %v", err)
	}
}

func TestContextBridge(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	policy := &SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
	}
	if err := mgr.AddSharingPolicy(policy); err != nil {
		t.Fatalf("failed to add policy: %v", err)
	}

	bridge := NewContextBridge(mgr)

	engine1, _ := mgr.GetEngine("session-1")
	ctx := context.Background()

	docs := []ctxpkg.Document{
		{ID: "doc-1", Content: "shared document 1"},
		{ID: "doc-2", Content: "shared document 2"},
	}

	for _, doc := range docs {
		if err := engine1.AddDocument(ctx, doc); err != nil {
			t.Fatalf("failed to add document: %v", err)
		}
	}

	transfer, err := bridge.TransferDocuments(ctx, "session-1", "session-2", []string{"doc-1", "doc-2"}, "")
	if err != nil {
		t.Fatalf("transfer failed: %v", err)
	}

	if transfer.Status != TransferStatusCompleted {
		t.Errorf("expected transfer status 'completed', got '%s'", transfer.Status)
	}

	if len(transfer.Documents) != 2 {
		t.Errorf("expected 2 transferred documents, got %d", len(transfer.Documents))
	}

	engine2, _ := mgr.GetEngine("session-2")
	docCount := engine2.DocumentCount()
	if docCount != 2 {
		t.Errorf("expected 2 documents in target engine, got %d", docCount)
	}
}

func TestContextAggregator(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	engine1, _ := mgr.GetEngine("session-1")
	engine2, _ := mgr.GetEngine("session-2")
	ctx := context.Background()

	engine1.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "agent 1 document about Go"})
	engine2.AddDocument(ctx, ctxpkg.Document{ID: "doc-2", Content: "agent 2 document about Python"})

	aggregator := NewContextAggregator(mgr)

	docs, err := aggregator.AggregateDocuments([]string{"session-1", "session-2"}, "")
	if err != nil {
		t.Fatalf("aggregation failed: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("expected 2 aggregated documents, got %d", len(docs))
	}
}

func TestContextSync(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	policy := &SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
	}
	if err := mgr.AddSharingPolicy(policy); err != nil {
		t.Fatalf("failed to add policy: %v", err)
	}

	engine1, _ := mgr.GetEngine("session-1")
	ctx := context.Background()
	engine1.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "sync document"})

	sync := NewContextSync(mgr)

	if err := sync.SyncContext("session-1", "session-2", ""); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	engine2, _ := mgr.GetEngine("session-2")
	docCount := engine2.DocumentCount()
	if docCount != 1 {
		t.Errorf("expected 1 document after sync, got %d", docCount)
	}
}

func TestIsolatedEngineClone(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	engine, _ := mgr.GetEngine("session-1")
	ctx := context.Background()

	engine.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "original document"})

	cloned := engine.Clone()

	if cloned.Scope().AgentID != "agent-1" {
		t.Errorf("expected cloned agent ID 'agent-1', got '%s'", cloned.Scope().AgentID)
	}

	if cloned.Visibility() != engine.Visibility() {
		t.Errorf("expected cloned visibility to match original")
	}

	docCount := cloned.(*IsolatedEngine).DocumentCount()
	if docCount != 1 {
		t.Errorf("expected 1 document in clone, got %d", docCount)
	}
}

func TestIsolatedEngineMaxSize(t *testing.T) {
	config := DefaultIsolationConfig()
	config.MaxContextSize = 2
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	engine, _ := mgr.GetEngine("session-1")
	ctx := context.Background()

	engine.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "doc 1"})
	engine.AddDocument(ctx, ctxpkg.Document{ID: "doc-2", Content: "doc 2"})

	err = engine.AddDocument(ctx, ctxpkg.Document{ID: "doc-3", Content: "doc 3"})
	if err == nil {
		t.Fatal("expected error when exceeding max size")
	}
}

func TestIsolatedEngineClosed(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}

	_, err := mgr.CreateBoundary(scope, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	engine, _ := mgr.GetEngine("session-1")
	ctx := context.Background()

	engine.Close()

	err = engine.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "test"})
	if err == nil {
		t.Fatal("expected error when adding to closed engine")
	}

	_, err = engine.GetDocument(ctx, "doc-1")
	if err == nil {
		t.Fatal("expected error when getting from closed engine")
	}
}

func TestKVEngineIsolation(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	kvEngine1, _ := mgr.GetKVEngine("session-1")
	kvEngine2, _ := mgr.GetKVEngine("session-2")
	ctx := context.Background()

	kvEngine1.Set(ctx, "key1", "value1")

	_, err = kvEngine2.Get(ctx, "key1")
	if err == nil {
		t.Fatal("expected KV to be isolated")
	}
}

func TestListSharingPolicies(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	policy1 := &SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
	}
	policy2 := &SharedContextPolicy{
		SourceAgentID:  "agent-3",
		TargetAgentIDs: []string{"agent-4"},
	}

	mgr.AddSharingPolicy(policy1)
	mgr.AddSharingPolicy(policy2)

	policies := mgr.ListSharingPolicies()
	if len(policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(policies))
	}
}

func TestContextScopeMiddlewareWithScope(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)

	var executedScopeID string
	err := middleware.WithScope(context.Background(), "agent-1", "session-1", "", "test", IsolationModeStrict, ContextVisibilityPrivate, func(scopeID string) error {
		executedScopeID = scopeID
		return nil
	})

	if err != nil {
		t.Fatalf("WithScope failed: %v", err)
	}

	if executedScopeID != "session-1" {
		t.Errorf("expected scope ID 'session-1', got '%s'", executedScopeID)
	}

	if middleware.IsScoped("agent-1") {
		t.Error("expected agent to be unscoped after WithScope")
	}
}

func TestContextEnforcerNoScope(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)
	enforcer := NewContextEnforcer(middleware)

	enforcer.AddRule("agent-1", &EnforcementRule{
		AgentID:            "agent-1",
		RequiredMode:       IsolationModeStrict,
		RequiredVisibility: ContextVisibilityPrivate,
	})

	err := enforcer.Enforce("agent-1")
	if err == nil {
		t.Fatal("expected enforcement to fail when no scope is active")
	}
}

func TestContextBridgeTransferPrivateSource(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeShared, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	policy := &SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
	}
	mgr.AddSharingPolicy(policy)

	bridge := NewContextBridge(mgr)
	ctx := context.Background()

	transfer, err := bridge.TransferDocuments(ctx, "session-1", "session-2", []string{"doc-1"}, "")
	if err != nil {
		t.Fatalf("transfer should not error: %v", err)
	}

	if transfer.Status != TransferStatusRejected {
		t.Errorf("expected transfer to be rejected due to private visibility, got '%s'", transfer.Status)
	}
}

func TestContextAggregatorCrossScopeSearch(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}

	_, err := mgr.CreateBoundary(scope1, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 1: %v", err)
	}

	_, err = mgr.CreateBoundary(scope2, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create boundary 2: %v", err)
	}

	engine1, _ := mgr.GetEngine("session-1")
	engine2, _ := mgr.GetEngine("session-2")
	ctx := context.Background()

	engine1.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "Go programming is great"})
	engine2.AddDocument(ctx, ctxpkg.Document{ID: "doc-2", Content: "Python for data science"})

	aggregator := NewContextAggregator(mgr)

	results, err := aggregator.CrossScopeSearch([]string{"session-1", "session-2"}, "Go programming", "", 5)
	if err != nil {
		t.Fatalf("cross-scope search failed: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("expected at least 1 result, got %d", len(results))
	}
}

func TestContextSyncAllPairs(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	scope1 := &ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}
	scope2 := &ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}
	scope3 := &ContextScope{
		AgentID:   "agent-3",
		SessionID: "session-3",
	}

	mgr.CreateBoundary(scope1, IsolationModeShared, ContextVisibilityPublic)
	mgr.CreateBoundary(scope2, IsolationModeShared, ContextVisibilityPublic)
	mgr.CreateBoundary(scope3, IsolationModeShared, ContextVisibilityPublic)

	mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2", "agent-3"},
	})
	mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:  "agent-2",
		TargetAgentIDs: []string{"agent-3"},
	})

	engine1, _ := mgr.GetEngine("session-1")
	ctx := context.Background()
	engine1.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "sync test"})

	sync := NewContextSync(mgr)

	results := sync.SyncAllPairs([]string{"session-1", "session-2", "session-3"}, "")

	if len(results) != 6 {
		t.Errorf("expected 6 sync results, got %d", len(results))
	}
}

func TestCreateChildBoundaryNilScope(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create parent boundary: %v", err)
	}

	_, err = mgr.CreateChildBoundary("session-1", nil, IsolationModeStrict, ContextVisibilityPrivate)
	if err == nil {
		t.Fatal("expected error when child scope is nil")
	}
}

func TestIsolatedEngineReturnsClones(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	engine, _ := mgr.GetEngine("session-1")
	ctx := context.Background()

	metadata := map[string]any{"type": "test"}
	vector := []float64{1, 2, 3}
	doc := ctxpkg.Document{
		ID:       "doc-1",
		Content:  "Go programming language",
		Metadata: metadata,
		Vector:   vector,
	}

	if err := engine.AddDocument(ctx, doc); err != nil {
		t.Fatalf("failed to add document: %v", err)
	}

	metadata["type"] = "mutated"
	vector[0] = 99

	stored, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("failed to get stored document: %v", err)
	}
	if stored.Metadata["type"] != "test" {
		t.Fatalf("expected stored metadata to stay isolated, got %v", stored.Metadata["type"])
	}
	if len(stored.Vector) != 3 || stored.Vector[0] != 1 {
		t.Fatalf("expected stored vector to stay isolated, got %#v", stored.Vector)
	}

	stored.Metadata["type"] = "changed"
	stored.Vector[0] = 77

	again, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("failed to get document again: %v", err)
	}
	if again.Metadata["type"] != "test" {
		t.Fatalf("expected GetDocument to return a clone, got %v", again.Metadata["type"])
	}
	if again.Vector[0] != 1 {
		t.Fatalf("expected GetDocument vector clone, got %#v", again.Vector)
	}

	results, err := engine.Search(ctx, "Go programming", ctxpkg.SearchOptions{
		TopK:      1,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}

	results[0].Document.Metadata["type"] = "search"
	postSearch, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("failed to get document after search mutation: %v", err)
	}
	if postSearch.Metadata["type"] != "test" {
		t.Fatalf("expected search result mutation to not leak, got %v", postSearch.Metadata["type"])
	}

	snapshot := engine.SnapshotDocuments()
	if len(snapshot) != 1 {
		t.Fatalf("expected 1 document in snapshot, got %d", len(snapshot))
	}
	snapshot[0].Metadata["type"] = "snapshot"
	snapshot[0].Vector[0] = 55

	postSnapshot, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("failed to get document after snapshot mutation: %v", err)
	}
	if postSnapshot.Metadata["type"] != "test" {
		t.Fatalf("expected snapshot mutation to not leak, got %v", postSnapshot.Metadata["type"])
	}
	if postSnapshot.Vector[0] != 1 {
		t.Fatalf("expected snapshot vector mutation to not leak, got %#v", postSnapshot.Vector)
	}
}

func TestSearchDocumentsTopKZeroMeansUnlimited(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create boundary: %v", err)
	}

	engine, _ := mgr.GetEngine("session-1")
	ctx := context.Background()

	for _, doc := range []ctxpkg.Document{
		{ID: "doc-1", Content: "Go programming"},
		{ID: "doc-2", Content: "Go concurrency"},
	} {
		if err := engine.AddDocument(ctx, doc); err != nil {
			t.Fatalf("failed to add document %s: %v", doc.ID, err)
		}
	}

	results, err := engine.Search(ctx, "Go", ctxpkg.SearchOptions{
		TopK:      0,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected unlimited TopK to return 2 results, got %d", len(results))
	}
}

func TestCanShareContextRequiresApproval(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	if err := mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:   "agent-1",
		TargetAgentIDs:  []string{"agent-2"},
		RequireApproval: true,
	}); err != nil {
		t.Fatalf("failed to add sharing policy: %v", err)
	}

	if mgr.CanShareContext("agent-1", "agent-2", "") {
		t.Fatal("expected approval-required policy to block automatic sharing")
	}
}

func TestSharedSearchRespectsContextKeys(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create source boundary: %v", err)
	}

	_, err = mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create target boundary: %v", err)
	}

	if err := mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
		ContextKeys:    []string{"allowed"},
	}); err != nil {
		t.Fatalf("failed to add sharing policy: %v", err)
	}

	kvEngine, _ := mgr.GetKVEngine("session-1")
	ctx := context.Background()
	if err := kvEngine.Set(ctx, "allowed", "value-1"); err != nil {
		t.Fatalf("failed to set allowed key: %v", err)
	}
	if err := kvEngine.Set(ctx, "blocked", "value-2"); err != nil {
		t.Fatalf("failed to set blocked key: %v", err)
	}

	results, err := mgr.SharedSearch("session-2", "", "")
	if err != nil {
		t.Fatalf("shared search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 shared result, got %d", len(results))
	}
	if results[0].Key != "allowed" {
		t.Fatalf("expected only allowed key to be shared, got %s", results[0].Key)
	}
}

func TestContextBridgeTransferRequiresApproval(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create source boundary: %v", err)
	}

	_, err = mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create target boundary: %v", err)
	}

	if err := mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:   "agent-1",
		TargetAgentIDs:  []string{"agent-2"},
		RequireApproval: true,
	}); err != nil {
		t.Fatalf("failed to add sharing policy: %v", err)
	}

	bridge := NewContextBridge(mgr)
	if _, err := bridge.TransferDocuments(context.Background(), "session-1", "session-2", []string{"doc-1"}, ""); err == nil {
		t.Fatal("expected approval-required policy to block transfer")
	}
}

func TestContextBridgeTransferKeyValuesRespectsContextKeys(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create source boundary: %v", err)
	}

	_, err = mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create target boundary: %v", err)
	}

	if err := mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
		ContextKeys:    []string{"allowed"},
	}); err != nil {
		t.Fatalf("failed to add sharing policy: %v", err)
	}

	sourceKV, _ := mgr.GetKVEngine("session-1")
	targetKV, _ := mgr.GetKVEngine("session-2")
	ctx := context.Background()
	if err := sourceKV.Set(ctx, "allowed", "value-1"); err != nil {
		t.Fatalf("failed to set allowed key: %v", err)
	}
	if err := sourceKV.Set(ctx, "blocked", "value-2"); err != nil {
		t.Fatalf("failed to set blocked key: %v", err)
	}

	bridge := NewContextBridge(mgr)
	transfer, err := bridge.TransferKeyValues("session-1", "session-2", []string{"allowed", "blocked"}, "")
	if err != nil {
		t.Fatalf("transfer failed: %v", err)
	}
	if len(transfer.KeyValues) != 1 {
		t.Fatalf("expected only 1 allowed key to transfer, got %d", len(transfer.KeyValues))
	}

	if _, err := targetKV.Get(ctx, "shared_session-1_allowed"); err != nil {
		t.Fatalf("expected allowed key to be transferred: %v", err)
	}
	if _, err := targetKV.Get(ctx, "shared_session-1_blocked"); err == nil {
		t.Fatal("expected blocked key to stay filtered out")
	}
}

func TestCreateSnapshotAggregatesAgentScopes(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	config.EnableSnapshots = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create session boundary: %v", err)
	}

	_, err = mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-2",
		TaskID:    "task-1",
	}, IsolationModeStrict, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create task boundary: %v", err)
	}

	sessionEngine, _ := mgr.GetEngine("session-1")
	taskEngine, _ := mgr.GetEngine("task-1")
	ctx := context.Background()
	if err := sessionEngine.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "session document"}); err != nil {
		t.Fatalf("failed to add session document: %v", err)
	}
	if err := taskEngine.AddDocument(ctx, ctxpkg.Document{ID: "doc-2", Content: "task document"}); err != nil {
		t.Fatalf("failed to add task document: %v", err)
	}

	sessionKV, _ := mgr.GetKVEngine("session-1")
	taskKV, _ := mgr.GetKVEngine("task-1")
	if err := sessionKV.Set(ctx, "key-1", "value-1"); err != nil {
		t.Fatalf("failed to set session key: %v", err)
	}
	if err := taskKV.Set(ctx, "key-2", "value-2"); err != nil {
		t.Fatalf("failed to set task key: %v", err)
	}

	snapshot, err := mgr.CreateSnapshot("agent-1", "", "aggregate")
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}
	if len(snapshot.Documents) != 2 {
		t.Fatalf("expected 2 documents in aggregate snapshot, got %d", len(snapshot.Documents))
	}
	if len(snapshot.KeyValues) != 2 {
		t.Fatalf("expected 2 key-values in aggregate snapshot, got %d", len(snapshot.KeyValues))
	}
}

func TestContextScopeMiddlewareExitNestedCleansUpBoundary(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)

	if _, err := middleware.EnterScope("agent-1", "session-1", "", "outer", IsolationModeStrict, ContextVisibilityPrivate); err != nil {
		t.Fatalf("failed to enter outer scope: %v", err)
	}
	if _, err := middleware.EnterScope("agent-1", "session-1", "task-1", "inner", IsolationModeHybrid, ContextVisibilityScoped); err != nil {
		t.Fatalf("failed to enter inner scope: %v", err)
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit inner scope: %v", err)
	}

	if _, ok := mgr.GetBoundary("task-1"); ok {
		t.Fatal("expected exited nested scope boundary to be deleted")
	}
	if _, ok := mgr.GetEngine("task-1"); ok {
		t.Fatal("expected exited nested scope engine to be deleted")
	}

	currentScope, err := middleware.GetCurrentScope("agent-1")
	if err != nil {
		t.Fatalf("failed to get current scope after nested exit: %v", err)
	}
	if currentScope.ID() != "session-1" {
		t.Fatalf("expected current scope to return to outer scope, got %s", currentScope.ID())
	}
}

func TestContextScopeMiddlewareExitNestedSameScopeKeepsBoundary(t *testing.T) {
	config := DefaultIsolationConfig()
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	middleware := NewContextScopeMiddleware(mgr)

	if _, err := middleware.EnterScope("agent-1", "session-1", "", "test", IsolationModeStrict, ContextVisibilityPrivate); err != nil {
		t.Fatalf("failed to enter first scope: %v", err)
	}
	if _, err := middleware.EnterScope("agent-1", "session-1", "", "test", IsolationModeStrict, ContextVisibilityPrivate); err != nil {
		t.Fatalf("failed to enter duplicate scope: %v", err)
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit nested duplicate scope: %v", err)
	}

	if _, ok := mgr.GetBoundary("session-1"); !ok {
		t.Fatal("expected boundary to remain while outer scope is still active")
	}
	if !middleware.IsScoped("agent-1") {
		t.Fatal("expected agent to stay scoped after first exit")
	}

	if err := middleware.ExitScope("agent-1"); err != nil {
		t.Fatalf("failed to exit outer scope: %v", err)
	}
	if _, ok := mgr.GetBoundary("session-1"); ok {
		t.Fatal("expected boundary to be deleted after final exit")
	}
}

func TestContextAggregatorCrossScopeSearchAllNamespaces(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
		Namespace: "alpha",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create first boundary: %v", err)
	}

	_, err = mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
		Namespace: "beta",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create second boundary: %v", err)
	}

	engine1, _ := mgr.GetEngine("session-1")
	engine2, _ := mgr.GetEngine("session-2")
	ctx := context.Background()
	if err := engine1.AddDocument(ctx, ctxpkg.Document{ID: "doc-1", Content: "Go programming"}); err != nil {
		t.Fatalf("failed to add first document: %v", err)
	}
	if err := engine2.AddDocument(ctx, ctxpkg.Document{ID: "doc-2", Content: "Go concurrency"}); err != nil {
		t.Fatalf("failed to add second document: %v", err)
	}

	aggregator := NewContextAggregator(mgr)
	results, err := aggregator.CrossScopeSearch([]string{"session-1", "session-2"}, "Go", "", 0)
	if err != nil {
		t.Fatalf("cross-scope search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected search across all namespaces to return 2 results, got %d", len(results))
	}
}

func TestContextSyncRejectsPrivateSource(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeShared, ContextVisibilityPrivate)
	if err != nil {
		t.Fatalf("failed to create private source boundary: %v", err)
	}

	_, err = mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create target boundary: %v", err)
	}

	if err := mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
	}); err != nil {
		t.Fatalf("failed to add sharing policy: %v", err)
	}

	sync := NewContextSync(mgr)
	if err := sync.SyncContext("session-1", "session-2", ""); err == nil {
		t.Fatal("expected sync from private source to be rejected")
	}
}

func TestContextSyncRespectsContextKeys(t *testing.T) {
	config := DefaultIsolationConfig()
	config.EnableSharing = true
	mgr := NewContextIsolationManager(config)
	defer mgr.Close()

	_, err := mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-1",
		SessionID: "session-1",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create source boundary: %v", err)
	}

	_, err = mgr.CreateBoundary(&ContextScope{
		AgentID:   "agent-2",
		SessionID: "session-2",
	}, IsolationModeShared, ContextVisibilityPublic)
	if err != nil {
		t.Fatalf("failed to create target boundary: %v", err)
	}

	if err := mgr.AddSharingPolicy(&SharedContextPolicy{
		SourceAgentID:  "agent-1",
		TargetAgentIDs: []string{"agent-2"},
		ContextKeys:    []string{"doc-1", "key-1"},
	}); err != nil {
		t.Fatalf("failed to add sharing policy: %v", err)
	}

	sourceEngine, _ := mgr.GetEngine("session-1")
	targetEngine, _ := mgr.GetEngine("session-2")
	sourceKV, _ := mgr.GetKVEngine("session-1")
	targetKV, _ := mgr.GetKVEngine("session-2")
	ctx := context.Background()

	for _, doc := range []ctxpkg.Document{
		{ID: "doc-1", Content: "allowed document"},
		{ID: "doc-2", Content: "blocked document"},
	} {
		if err := sourceEngine.AddDocument(ctx, doc); err != nil {
			t.Fatalf("failed to add document %s: %v", doc.ID, err)
		}
	}
	if err := sourceKV.Set(ctx, "key-1", "value-1"); err != nil {
		t.Fatalf("failed to set allowed key: %v", err)
	}
	if err := sourceKV.Set(ctx, "key-2", "value-2"); err != nil {
		t.Fatalf("failed to set blocked key: %v", err)
	}

	sync := NewContextSync(mgr)
	if err := sync.SyncContext("session-1", "session-2", ""); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if targetEngine.DocumentCount() != 1 {
		t.Fatalf("expected only 1 allowed document to sync, got %d", targetEngine.DocumentCount())
	}
	if _, err := targetEngine.GetDocument(ctx, "doc-1"); err != nil {
		t.Fatalf("expected allowed document to sync: %v", err)
	}
	if _, err := targetEngine.GetDocument(ctx, "doc-2"); err == nil {
		t.Fatal("expected blocked document to stay out of sync target")
	}

	if _, err := targetKV.Get(ctx, "key-1"); err != nil {
		t.Fatalf("expected allowed key to sync: %v", err)
	}
	if _, err := targetKV.Get(ctx, "key-2"); err == nil {
		t.Fatal("expected blocked key to stay out of sync target")
	}
}
