package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/clawbridge"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
	"github.com/anyclaw/anyclaw/pkg/workspace"
)

type stubAgentLLM struct {
	responses []*llm.Response
	index     int
	messages  [][]llm.Message
}

func (s *stubAgentLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	s.messages = append(s.messages, append([]llm.Message(nil), messages...))
	if s.index >= len(s.responses) {
		return &llm.Response{Content: "done"}, nil
	}
	resp := s.responses[s.index]
	s.index++
	return resp, nil
}

func (s *stubAgentLLM) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	resp, err := s.Chat(ctx, messages, toolDefs)
	if err != nil {
		return err
	}
	if resp != nil && onChunk != nil {
		onChunk(resp.Content)
	}
	return nil
}

func (s *stubAgentLLM) Name() string {
	return "stub"
}

func TestBuildSystemPromptIncludesPersonalityAndAnyClawCore(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("app_qq_workflow_send_message", "Send a message through QQ", map[string]any{}, nil)
	registry.RegisterTool("desktop_resolve_target", "Resolve a local app target", map[string]any{}, nil)
	registry.RegisterTool("desktop_activate_target", "Activate a local app target", map[string]any{}, nil)
	registry.RegisterTool("desktop_set_target_value", "Set a value on a local app target", map[string]any{}, nil)
	registry.RegisterTool("desktop_wait_text", "Wait for local app text", map[string]any{}, nil)

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Operate like an execution-focused local app agent.",
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if !strings.Contains(systemPrompt, "execution-focused local app agent") {
		t.Fatalf("expected personality to be injected into the system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "## AnyClaw Core") {
		t.Fatalf("expected AnyClaw core operating section, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "app_qq_workflow_send_message") {
		t.Fatalf("expected workflow tool guidance in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "desktop_activate_target") || !strings.Contains(systemPrompt, "desktop_set_target_value") {
		t.Fatalf("expected target-based desktop guidance in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "Work in loops: inspect the current state") {
		t.Fatalf("expected iterative execution guidance in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "verify the requested deliverable with observable evidence") {
		t.Fatalf("expected verification guidance in system prompt, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptHandlesNilDependencies(t *testing.T) {
	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Stay calm and action-oriented.",
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if !strings.Contains(systemPrompt, "Stay calm and action-oriented.") {
		t.Fatalf("expected personality text in prompt, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "## AnyClaw Core") {
		t.Fatalf("did not expect AnyClaw core section without execution tools, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptInjectsWorkspaceBootstrapFiles(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	workingDir := t.TempDir()
	if err := workspace.EnsureBootstrap(workingDir, workspace.BootstrapOptions{
		AgentName:        "assistant",
		AgentDescription: "Local execution helper",
	}); err != nil {
		t.Fatalf("EnsureBootstrap: %v", err)
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Operate like an execution-focused local app agent.",
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       tools.NewRegistry(),
		WorkingDir:  workingDir,
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if !strings.Contains(systemPrompt, "## Workspace") || !strings.Contains(systemPrompt, workingDir) {
		t.Fatalf("expected workspace section in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "## Project Context") {
		t.Fatalf("expected project context section in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "### AGENTS.md") || !strings.Contains(systemPrompt, "### MEMORY.md") {
		t.Fatalf("expected bootstrap files to be injected, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptInjectsCLIHubContext(t *testing.T) {
	hubRoot := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	if err := writeCLIHubFixture(hubRoot); err != nil {
		t.Fatalf("writeCLIHubFixture: %v", err)
	}
	t.Setenv("ANYCLAW_CLI_ANYTHING_ROOT", hubRoot)

	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("clihub_catalog", "Search local CLI Hub entries", map[string]any{}, nil)
	registry.RegisterTool("clihub_exec", "Execute local CLI Hub entries", map[string]any{}, nil)

	workingDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Operate like an execution-focused local app agent.",
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		WorkingDir:  workingDir,
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if !strings.Contains(systemPrompt, "## CLI Hub") {
		t.Fatalf("expected CLI Hub section in prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, hubRoot) {
		t.Fatalf("expected hub root in prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "office (2)") || !strings.Contains(systemPrompt, "video (1)") {
		t.Fatalf("expected category summary in prompt, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptInjectsClawBridgeContext(t *testing.T) {
	bridgeRoot := filepath.Join(t.TempDir(), "claw-code-main")
	if err := writeAgentBridgeFixture(bridgeRoot); err != nil {
		t.Fatalf("writeAgentBridgeFixture: %v", err)
	}
	t.Setenv(clawbridge.EnvRoot, bridgeRoot)

	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, nil)

	workingDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Operate like an execution-focused local app agent.",
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		WorkingDir:  workingDir,
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if !strings.Contains(systemPrompt, "## Claw Bridge") {
		t.Fatalf("expected claw bridge section in prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, bridgeRoot) {
		t.Fatalf("expected bridge root in prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "agents (2)") || !strings.Contains(systemPrompt, "AgentTool (2)") {
		t.Fatalf("expected command and tool family summaries in prompt, got %q", systemPrompt)
	}
}

func TestAgentRunExecutesProtocolPlanAndReturnsFollowupResponse(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	var called []string
	registry.RegisterTool("desktop_activate_target", "Activate a target", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, fmt.Sprintf("%v:%v", input["title"], input["text"]))
		return "clicked", nil
	})

	llmStub := &stubAgentLLM{responses: []*llm.Response{
		{Content: "```json\n{\"protocol\":\"anyclaw.app.desktop.v1\",\"summary\":\"plan complete\",\"steps\":[{\"label\":\"Click send\",\"target\":{\"title\":\"QQ\",\"text\":\"发送\"}}]}\n```"},
		{Content: "已经完成了，本地应用里的发送步骤执行成功。"},
	}}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Operate like an execution-focused local app agent.",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	var approvals []tools.ToolApprovalCall
	ctx := tools.WithToolApprovalHook(context.Background(), func(ctx context.Context, call tools.ToolApprovalCall) error {
		approvals = append(approvals, call)
		return nil
	})

	result, err := ag.Run(ctx, "帮我在QQ里发送消息")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "已经完成了，本地应用里的发送步骤执行成功。" {
		t.Fatalf("unexpected final result: %q", result)
	}
	if strings.Join(called, ",") != "QQ:发送" {
		t.Fatalf("expected protocol step to activate QQ send target, got %v", called)
	}
	if len(approvals) != 1 || approvals[0].Name != "desktop_plan" {
		t.Fatalf("expected one desktop_plan approval, got %#v", approvals)
	}
	if len(llmStub.messages) < 2 {
		t.Fatalf("expected follow-up LLM turn after plan execution, got %d message batches", len(llmStub.messages))
	}
	foundExecutionResult := false
	for _, msg := range llmStub.messages[1] {
		if msg.Role == "user" && strings.Contains(msg.Content, "Desktop plan execution result:") && strings.Contains(msg.Content, "Click send: clicked") && strings.Contains(msg.Content, "Treat this as observable evidence") {
			foundExecutionResult = true
			break
		}
	}
	if !foundExecutionResult {
		t.Fatalf("expected second LLM turn to receive desktop plan result, got %#v", llmStub.messages[1])
	}
}

func TestAgentRunAddsObservationAndVerificationPromptAfterToolResults(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "build succeeded", nil
	})

	llmStub := &stubAgentLLM{responses: []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"go test ./..."}`,
					},
				},
			},
		},
		{Content: "测试已完成并验证通过。"},
	}}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Operate like an execution-focused local app agent.",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	result, err := ag.Run(context.Background(), "帮我跑测试并确认是否通过")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "测试已完成并验证通过。" {
		t.Fatalf("unexpected result: %q", result)
	}
	if len(llmStub.messages) < 2 {
		t.Fatalf("expected second llm turn after tool call, got %d", len(llmStub.messages))
	}
	foundFollowup := false
	for _, msg := range llmStub.messages[1] {
		if msg.Role == "user" && strings.Contains(msg.Content, "Tool results above are evidence about the current world state") && strings.Contains(msg.Content, "Before claiming completion, verify the outcome") {
			foundFollowup = true
			break
		}
	}
	if !foundFollowup {
		t.Fatalf("expected observation/verification follow-up prompt, got %#v", llmStub.messages[1])
	}
}

func writeAgentBridgeFixture(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "src", "reference_data", "subsystems"), 0o755); err != nil {
		return err
	}
	commands := []map[string]string{
		{"name": "agents", "source_hint": "commands/agents/index.ts"},
		{"name": "agents", "source_hint": "commands/agents/agents.tsx"},
		{"name": "tasks", "source_hint": "commands/tasks/index.ts"},
	}
	toolItems := []map[string]string{
		{"name": "AgentTool", "source_hint": "tools/AgentTool/AgentTool.tsx"},
		{"name": "agentMemory", "source_hint": "tools/AgentTool/agentMemory.ts"},
		{"name": "ReadFileTool", "source_hint": "tools/ReadFileTool/ReadFileTool.tsx"},
	}
	subsystem := map[string]any{
		"archive_name": "assistant",
		"module_count": 12,
		"sample_files": []string{"assistant/sessionHistory.ts"},
	}
	if err := writeAgentJSON(filepath.Join(root, "src", "reference_data", "commands_snapshot.json"), commands); err != nil {
		return err
	}
	if err := writeAgentJSON(filepath.Join(root, "src", "reference_data", "tools_snapshot.json"), toolItems); err != nil {
		return err
	}
	return writeAgentJSON(filepath.Join(root, "src", "reference_data", "subsystems", "assistant.json"), subsystem)
}

func writeAgentJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeCLIHubFixture(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut", "__main__.py"), []byte("print('ok')"), 0o644); err != nil {
		return err
	}
	payload := map[string]any{
		"meta": map[string]any{
			"repo":        "https://example.com/CLI-Anything",
			"description": "CLI-Hub",
			"updated":     "2026-03-29",
		},
		"clis": []map[string]any{
			{"name": "libreoffice", "display_name": "LibreOffice", "description": "Office suite", "category": "office", "entry_point": "cli-anything-libreoffice"},
			{"name": "zotero", "display_name": "Zotero", "description": "References", "category": "office", "entry_point": "cli-anything-zotero"},
			{"name": "shotcut", "display_name": "Shotcut", "description": "Video editing", "category": "video", "entry_point": "cli-anything-shotcut"},
		},
	}
	return writeAgentJSON(filepath.Join(root, "registry.json"), payload)
}

func TestAgentRunCompletesBootstrapRitualBeforeCallingLLM(t *testing.T) {
	workDir := t.TempDir()
	mem := memory.NewFileMemory(workDir)
	mem.SetDailyDir(filepath.Join(workDir, "workspace", "memory"))
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	workingDir := filepath.Join(workDir, "workspace")
	if err := workspace.EnsureBootstrap(workingDir, workspace.BootstrapOptions{
		AgentName:        "assistant",
		AgentDescription: "Local execution helper",
	}); err != nil {
		t.Fatalf("EnsureBootstrap: %v", err)
	}

	llmStub := &stubAgentLLM{responses: []*llm.Response{{Content: "normal task response"}}}
	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       tools.NewRegistry(),
		WorkingDir:  workingDir,
	})

	answer, err := ag.Run(context.Background(), "help me with this repo")
	if err != nil {
		t.Fatalf("Run(q1): %v", err)
	}
	if !strings.Contains(answer, "Question 1/4") {
		t.Fatalf("expected bootstrap question, got %q", answer)
	}
	if len(llmStub.messages) != 0 {
		t.Fatalf("expected llm not to be called during bootstrap, got %d calls", len(llmStub.messages))
	}

	sequence := []string{
		"Call me Alex and default to Chinese.",
		"Mainly help with Go coding and local automation.",
		"Be concise, proactive, and optimize for correctness first.",
		"Do not use destructive commands without explicit confirmation.",
	}
	for i, input := range sequence {
		answer, err = ag.Run(context.Background(), input)
		if err != nil {
			t.Fatalf("Run(answer %d): %v", i+1, err)
		}
	}

	if !strings.Contains(answer, "Workspace bootstrap complete") {
		t.Fatalf("expected bootstrap completion message, got %q", answer)
	}
	if _, err := os.Stat(filepath.Join(workingDir, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("expected BOOTSTRAP.md to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".anyclaw-bootstrap-state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected bootstrap state file to be removed, stat err=%v", err)
	}

	identityData, err := os.ReadFile(filepath.Join(workingDir, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("ReadFile(IDENTITY.md): %v", err)
	}
	if !strings.Contains(string(identityData), "Mainly help with Go coding and local automation.") {
		t.Fatalf("expected IDENTITY.md to include bootstrap answer, got %q", string(identityData))
	}

	normalResponse, err := ag.Run(context.Background(), "now answer normally")
	if err != nil {
		t.Fatalf("Run(normal): %v", err)
	}
	if normalResponse != "normal task response" {
		t.Fatalf("expected normal llm response after bootstrap, got %q", normalResponse)
	}
	if len(llmStub.messages) != 1 {
		t.Fatalf("expected one llm call after bootstrap, got %d", len(llmStub.messages))
	}
}
