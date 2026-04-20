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
