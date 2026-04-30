package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	for _, toolName := range []string{"write", "edit", "apply_patch", "exec", "process", "write_file", "run_command"} {
		if isToolAllowedForPermission(toolName, "read-only") {
			t.Fatalf("expected mutation alias %s to be hidden from read-only agents", toolName)
		}
	}
}

func TestSubAgentSkillExecutionHonorsConfiguredExecPolicy(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "runner")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{
  "name": "runner",
  "description": "Runs external work",
  "version": "1.0.0",
  "entrypoint": "run.sh"
}`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	manager := skills.NewSkillsManager(root)
	if err := manager.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	def := AgentDefinition{
		Name:            "worker",
		Description:     "Uses private skills",
		PrivateSkills:   []string{"runner"},
		PermissionLevel: "limited",
	}
	sa, err := NewSubAgentWithContext(def, &stubSubAgentLLM{}, manager, tools.NewRegistry(), nil, nil, "", skills.ExecutionOptions{AllowExec: false})
	if err != nil {
		t.Fatalf("NewSubAgentWithContext: %v", err)
	}

	_, err = sa.tools.Call(context.Background(), "skill_runner", map[string]any{"action": "run"})
	if err == nil || !strings.Contains(err.Error(), "skill execution disabled") {
		t.Fatalf("expected private executable skill to honor AllowExec=false, got %v", err)
	}
}

func TestSubAgentDoesNotInheritSkillsWithoutPrivateSkills(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "planner")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{
  "name": "planner",
  "description": "Plans work",
  "version": "1.0.0",
  "prompts": {"system": "plan"}
}`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	manager := skills.NewSkillsManager(root)
	if err := manager.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	sa, err := NewSubAgentWithContext(AgentDefinition{
		Name:            "worker",
		Description:     "No private skills configured",
		PermissionLevel: "limited",
	}, &stubSubAgentLLM{}, manager, tools.NewRegistry(), nil, nil, "")
	if err != nil {
		t.Fatalf("NewSubAgentWithContext: %v", err)
	}
	if got := sa.Skills(); len(got) != 0 {
		t.Fatalf("expected no inherited skills without private_skills, got %#v", got)
	}
	if _, ok := sa.tools.Get("skill_planner"); ok {
		t.Fatal("expected planner skill tool to be absent")
	}
}

func TestSubAgentRebindsBuiltinToolsToAgentWorkingDir(t *testing.T) {
	root := t.TempDir()
	mainDir := filepath.Join(root, "main")
	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	baseTools := tools.NewRegistry()
	mainOpts := tools.BuiltinOptions{
		WorkingDir:      mainDir,
		PermissionLevel: "limited",
		Policy: tools.NewPolicyEngine(tools.PolicyOptions{
			WorkingDir:      mainDir,
			PermissionLevel: "limited",
		}),
	}
	tools.RegisterBuiltins(baseTools, mainOpts)

	sa, err := NewSubAgentWithRuntimeOptions(AgentDefinition{
		Name:            "worker",
		Description:     "Uses rebound builtins",
		PermissionLevel: "limited",
		WorkingDir:      subDir,
	}, &stubSubAgentLLM{}, skills.NewSkillsManager(""), baseTools, nil, nil, "", SubAgentRuntimeOptions{BuiltinTools: &mainOpts})
	if err != nil {
		t.Fatalf("NewSubAgentWithRuntimeOptions: %v", err)
	}

	ctx := tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleSubAgent, AgentName: "worker"})
	raw, err := sa.tools.Call(ctx, "session_status", map[string]any{})
	if err != nil {
		t.Fatalf("session_status: %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatalf("unmarshal status: %v\nraw=%s", err, raw)
	}
	if status["working_dir"] != subDir {
		t.Fatalf("expected sub-agent working_dir %q, got %#v", subDir, status["working_dir"])
	}
	if status["permission_level"] != "limited" {
		t.Fatalf("expected permission_level limited, got %#v", status["permission_level"])
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
