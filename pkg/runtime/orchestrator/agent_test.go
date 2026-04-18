package orchestrator

import (
	"context"
	"testing"

	agentpkg "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/isolation"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

type stubSubAgentLLM struct{}

func (s *stubSubAgentLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "sub-agent response"}, nil
}

func (s *stubSubAgentLLM) StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error {
	if onChunk != nil {
		onChunk("sub-agent response")
	}
	return nil
}

func (s *stubSubAgentLLM) Name() string { return "stub-sub-agent" }

func TestIsToolAllowedForPermissionReadOnlyDesktopTools(t *testing.T) {
	if !isToolAllowedForPermission("desktop_screenshot", "read-only") {
		t.Fatal("expected desktop_screenshot to remain available for read-only agents")
	}
	for _, toolName := range []string{"desktop_list_windows", "desktop_wait_window", "desktop_inspect_ui", "desktop_resolve_target", "desktop_match_image", "desktop_wait_image", "desktop_ocr", "desktop_verify_text", "desktop_find_text", "desktop_wait_text"} {
		if !isToolAllowedForPermission(toolName, "read-only") {
			t.Fatalf("expected %s to remain available for read-only agents", toolName)
		}
	}
	for _, toolName := range []string{"desktop_open", "desktop_type", "desktop_hotkey", "desktop_click"} {
		if isToolAllowedForPermission(toolName, "read-only") {
			t.Fatalf("expected %s to be hidden from read-only agents", toolName)
		}
	}
}

func TestNewSubAgentWithContextStoresConversationInIsolationEngine(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	manager := isolation.NewContextIsolationManager(isolation.DefaultIsolationConfig())
	t.Cleanup(func() {
		_ = manager.Close()
	})

	def := AgentDefinition{
		Name:            "researcher",
		Description:     "Investigates tasks",
		PermissionLevel: "limited",
		WorkingDir:      t.TempDir(),
	}

	sa, err := NewSubAgentWithContext(def, &stubSubAgentLLM{}, skills.NewSkillsManager(""), tools.NewRegistry(), mem, manager, "")
	if err != nil {
		t.Fatalf("NewSubAgentWithContext: %v", err)
	}
	t.Cleanup(func() { sa.memory.Close() })
	if !sa.HasIsolatedContext() {
		t.Fatal("expected isolated context engine to be attached")
	}

	result, err := sa.Run(context.Background(), "inspect the repository")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "sub-agent response" {
		t.Fatalf("unexpected result: %q", result)
	}

	docs := sa.ContextEngine().SnapshotDocuments()
	if len(docs) < 2 {
		t.Fatalf("expected user and assistant messages in isolated context, got %d", len(docs))
	}

	foundUser := false
	foundAssistant := false
	for _, doc := range docs {
		if role, _ := doc.Metadata["role"].(string); role == "user" {
			foundUser = true
		}
		if role, _ := doc.Metadata["role"].(string); role == "assistant" {
			foundAssistant = true
		}
		if agentID, _ := doc.Metadata["agent_id"].(string); agentID != "" && agentID != def.Name {
			t.Fatalf("expected isolated metadata agent_id=%s, got %v", def.Name, doc.Metadata["agent_id"])
		}
	}

	if !foundUser || !foundAssistant {
		t.Fatalf("expected both user and assistant documents, got %#v", docs)
	}

	if sa.agent == nil {
		t.Fatal("expected underlying agent to be created")
	}
	if _, ok := interface{}(sa.agent).(*agentpkg.Agent); !ok {
		t.Fatal("expected underlying type to remain *agent.Agent")
	}
}
