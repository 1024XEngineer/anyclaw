package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
