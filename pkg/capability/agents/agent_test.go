package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/prompt"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/clawbridge"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/workspace"
)

type stubAgentLLM struct {
	responses []*llm.Response
	index     int
	messages  [][]llm.Message
	toolDefs  [][]llm.ToolDefinition
}

func (s *stubAgentLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	s.messages = append(s.messages, append([]llm.Message(nil), messages...))
	s.toolDefs = append(s.toolDefs, append([]llm.ToolDefinition(nil), toolDefs...))
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

type compactionAwareLLM struct {
	mu       sync.Mutex
	messages [][]llm.Message
}

func (s *compactionAwareLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	s.mu.Lock()
	s.messages = append(s.messages, append([]llm.Message(nil), messages...))
	s.mu.Unlock()

	if len(messages) > 0 && messages[0].Role == "system" && strings.Contains(messages[0].Content, "conversation summarizer") {
		return &llm.Response{Content: "condensed prior work"}, nil
	}
	return &llm.Response{Content: "final response"}, nil
}

func (s *compactionAwareLLM) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	resp, err := s.Chat(ctx, messages, toolDefs)
	if err != nil {
		return err
	}
	if onChunk != nil {
		onChunk(resp.Content)
	}
	return nil
}

func (s *compactionAwareLLM) Name() string { return "compaction-aware" }

type blockingAgentLLM struct {
	mu      sync.Mutex
	entered chan int
	gates   []chan struct{}
}

func newBlockingAgentLLM() *blockingAgentLLM {
	return &blockingAgentLLM{
		entered: make(chan int, 4),
		gates:   make([]chan struct{}, 0, 4),
	}
}

func (s *blockingAgentLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	s.mu.Lock()
	idx := len(s.gates)
	gate := make(chan struct{})
	s.gates = append(s.gates, gate)
	s.mu.Unlock()

	s.entered <- idx

	select {
	case <-gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &llm.Response{Content: fmt.Sprintf("response-%d", idx)}, nil
}

func (s *blockingAgentLLM) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	resp, err := s.Chat(ctx, messages, toolDefs)
	if err != nil {
		return err
	}
	if onChunk != nil {
		onChunk(resp.Content)
	}
	return nil
}

func (s *blockingAgentLLM) Name() string { return "blocking" }

func (s *blockingAgentLLM) Release(idx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idx < 0 || idx >= len(s.gates) {
		return
	}
	select {
	case <-s.gates[idx]:
	default:
		close(s.gates[idx])
	}
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

func TestBuildSystemPromptLimitsInjectedMemory(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	for i := 0; i < 12; i++ {
		if err := mem.Add(memory.MemoryEntry{
			Type:      memory.TypeConversation,
			Timestamp: time.Unix(int64(i+1), 0),
			Content:   fmt.Sprintf("conversation-%02d %s", i, strings.Repeat("x", 900)),
		}); err != nil {
			t.Fatalf("add conversation memory %d: %v", i, err)
		}
	}
	if err := mem.Add(memory.MemoryEntry{
		Type:      memory.TypeFact,
		Timestamp: time.Unix(100, 0),
		Content:   "stable-fact " + strings.Repeat("y", 900),
	}); err != nil {
		t.Fatalf("add fact memory: %v", err)
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       tools.NewRegistry(),
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if len(systemPrompt) > 12000 {
		t.Fatalf("expected bounded prompt length, got %d characters", len(systemPrompt))
	}
	if !strings.Contains(systemPrompt, "stable-fact") {
		t.Fatalf("expected stable memory to be retained, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "conversation-11") || strings.Contains(systemPrompt, "conversation-00") {
		t.Fatalf("expected conversation memories to stay out of prompt injection, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "Prompt memory limited.") {
		t.Fatalf("expected truncated memory notice, got %q", systemPrompt)
	}
}

func TestSelectToolInfosSkipsBulkToolsForCasualQuestion(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("read_file", "Read a file", map[string]any{}, nil)
	registry.RegisterTool("desktop_open", "Open a desktop app", map[string]any{}, nil)
	registry.RegisterTool("browser_navigate", "Open a page", map[string]any{}, nil)
	registry.RegisterTool("blender_open", "Open Blender", map[string]any{}, nil)

	ag := New(Config{Tools: registry})

	casual := ag.selectToolInfos("你是谁")
	if len(casual) != 0 {
		t.Fatalf("expected no tools for casual question, got %#v", casual)
	}

	actionable := ag.selectToolInfos("请打开 blender 并读取文件")
	names := make([]string, 0, len(actionable))
	for _, tool := range actionable {
		names = append(names, tool.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "read_file") || !strings.Contains(got, "desktop_open") || !strings.Contains(got, "blender_open") {
		t.Fatalf("expected core and matched app tools, got %q", got)
	}
}

func TestSelectToolInfosHandlesChineseDesktopRequest(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("desktop_open", "Open a desktop app", map[string]any{}, nil)
	registry.RegisterTool("desktop_list_windows", "List desktop windows", map[string]any{}, nil)
	registry.RegisterTool("skill_app-controller", "Desktop app control guidance", map[string]any{}, nil)

	ag := New(Config{Tools: registry})

	actionable := ag.selectToolInfos("帮我打开steam")
	names := make([]string, 0, len(actionable))
	for _, tool := range actionable {
		names = append(names, tool.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "desktop_open") || !strings.Contains(got, "desktop_list_windows") || !strings.Contains(got, "skill_app-controller") {
		t.Fatalf("expected desktop and skill tools for Chinese open-app request, got %q", got)
	}
}

func TestSelectToolInfosHandlesChineseCreateFolderRequest(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("read_file", "Read a file", map[string]any{}, nil)
	registry.RegisterTool("write_file", "Write a file", map[string]any{}, nil)
	registry.RegisterTool("list_directory", "List a directory", map[string]any{}, nil)
	registry.RegisterTool("search_files", "Search files", map[string]any{}, nil)
	registry.RegisterTool("run_command", "Run a command", map[string]any{}, nil)

	ag := New(Config{Tools: registry})

	actionable := ag.selectToolInfos("在桌面创建文件夹哈喽")
	names := make([]string, 0, len(actionable))
	for _, tool := range actionable {
		names = append(names, tool.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "run_command") || !strings.Contains(got, "write_file") || !strings.Contains(got, "list_directory") {
		t.Fatalf("expected core file and command tools for Chinese create-folder request, got %q", got)
	}
}

func TestAgentRunUsesToolsForNaturalChineseCreateFolderRequest(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	registry := tools.NewRegistry()
	registry.RegisterTool("read_file", "Read a file", map[string]any{}, nil)
	registry.RegisterTool("write_file", "Write a file", map[string]any{}, nil)
	registry.RegisterTool("list_directory", "List a directory", map[string]any{}, nil)
	registry.RegisterTool("search_files", "Search files", map[string]any{}, nil)
	registry.RegisterTool("run_command", "Run a command", map[string]any{}, nil)

	llmStub := &stubAgentLLM{responses: []*llm.Response{
		{Content: "可以帮你创建。"},
	}}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	if _, err := ag.Run(context.Background(), "在桌面建立一个叫哈喽的文件夹"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(llmStub.messages) == 0 {
		t.Fatalf("expected llm to receive at least one message batch")
	}
	if len(llmStub.toolDefs) == 0 {
		t.Fatalf("expected llm to receive tool definitions")
	}

	systemPrompt := ""
	for _, msg := range llmStub.messages[0] {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			break
		}
	}
	if systemPrompt == "" {
		t.Fatalf("expected system prompt to be present")
	}
	if strings.Contains(systemPrompt, "No tools selected for this turn") {
		t.Fatalf("expected actionable Chinese request to expose tools, got system prompt %q", systemPrompt)
	}

	names := make([]string, 0, len(llmStub.toolDefs[0]))
	for _, def := range llmStub.toolDefs[0] {
		names = append(names, def.Function.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "run_command") || !strings.Contains(got, "write_file") || !strings.Contains(got, "list_directory") {
		t.Fatalf("expected run path to pass core tool definitions, got %q", got)
	}
}

func TestSelectToolInfosExposesCLIHubIntentToolsForNaturalLanguage(t *testing.T) {
	hubRoot := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	if err := writeCLIHubFixture(hubRoot); err != nil {
		t.Fatalf("writeCLIHubFixture: %v", err)
	}

	registry := tools.NewRegistry()
	registry.RegisterTool("intent_route", "Auto-route a CLI Hub request", map[string]any{}, nil)
	registry.RegisterTool("intent_list_capabilities", "List matching CLI Hub capabilities", map[string]any{}, nil)
	registry.RegisterTool("clihub_exec", "Execute a CLI Hub harness", map[string]any{}, nil)

	ag := New(Config{
		Tools:      registry,
		CLIHubRoot: hubRoot,
	})

	selected := ag.selectToolInfos("帮我用 shotcut 新建一个项目")
	names := make([]string, 0, len(selected))
	for _, tool := range selected {
		names = append(names, tool.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "intent_route") || !strings.Contains(got, "intent_list_capabilities") || !strings.Contains(got, "clihub_exec") {
		t.Fatalf("expected CLI Hub intent tools to be exposed, got %q", got)
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

func TestBuildSystemPromptHighlightsIntentRoutingWhenAvailable(t *testing.T) {
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
	registry.RegisterTool("intent_route", "Route a natural-language request", map[string]any{}, nil)
	registry.RegisterTool("intent_list_capabilities", "List matching capabilities", map[string]any{}, nil)
	registry.RegisterTool("clihub_exec", "Execute local CLI Hub entries", map[string]any{}, nil)

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		WorkingDir:  filepath.Join(t.TempDir(), "workspace"),
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if !strings.Contains(systemPrompt, "prefer intent_route first") {
		t.Fatalf("expected intent_route guidance in prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "intent_list_capabilities") {
		t.Fatalf("expected intent_list_capabilities guidance in prompt, got %q", systemPrompt)
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

func TestAgentRunAutoRoutesCLIHubIntentBeforeLLM(t *testing.T) {
	hubRoot := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	if err := writeCLIHubFixture(hubRoot); err != nil {
		t.Fatalf("writeCLIHubFixture: %v", err)
	}

	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	llmStub := &stubAgentLLM{responses: []*llm.Response{{Content: "llm fallback"}}}
	registry := tools.NewRegistry()
	var called bool
	var captured map[string]any
	registry.RegisterTool("intent_route", "Route a natural-language request", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		called = true
		captured = input
		return `{"status":"ok","command":"shotcut render"}`, nil
	})

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		CLIHubRoot:  hubRoot,
	})

	query := "export the current shotcut project"
	result, err := ag.Run(context.Background(), query)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != `{"status":"ok","command":"shotcut render"}` {
		t.Fatalf("unexpected result: %q", result)
	}
	if !called {
		t.Fatal("expected intent_route to be called")
	}
	if captured["intent"] != query || captured["json"] != true {
		t.Fatalf("unexpected intent_route input: %#v", captured)
	}
	if len(llmStub.messages) != 0 {
		t.Fatalf("expected llm to be skipped, got %d calls", len(llmStub.messages))
	}
	activities := ag.GetLastToolActivities()
	if len(activities) != 1 || activities[0].ToolName != "intent_route" {
		t.Fatalf("expected intent_route activity, got %#v", activities)
	}
}

func TestAgentRunFallsBackToLLMWhenAutoRouteFails(t *testing.T) {
	hubRoot := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	if err := writeCLIHubFixture(hubRoot); err != nil {
		t.Fatalf("writeCLIHubFixture: %v", err)
	}

	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	llmStub := &stubAgentLLM{responses: []*llm.Response{{Content: "fallback after route failure"}}}
	registry := tools.NewRegistry()
	registry.RegisterTool("intent_route", "Route a natural-language request", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "", fmt.Errorf("boom")
	})

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
		CLIHubRoot:  hubRoot,
	})

	result, err := ag.Run(context.Background(), "export the current shotcut project")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "fallback after route failure" {
		t.Fatalf("unexpected result: %q", result)
	}
	if len(llmStub.messages) != 1 {
		t.Fatalf("expected llm fallback after route failure, got %d calls", len(llmStub.messages))
	}
	activities := ag.GetLastToolActivities()
	if len(activities) == 0 || activities[0].ToolName != "intent_route" || activities[0].Error == "" {
		t.Fatalf("expected failed intent_route activity, got %#v", activities)
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
	shotcutRoot := filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut")
	if err := os.MkdirAll(filepath.Join(shotcutRoot, "skills"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(shotcutRoot, "__main__.py"), []byte("print('ok')"), 0o644); err != nil {
		return err
	}
	skill := `---
name: Shotcut CLI
description: Video editing harness
---

## Project Group
| Command | Description |
| --- | --- |
| new | Create a new project |
| info | Show current project info |

## Export Group
| Command | Description |
| --- | --- |
| render | Export the current project |
`
	if err := os.WriteFile(filepath.Join(shotcutRoot, "skills", "SKILL.md"), []byte(skill), 0o644); err != nil {
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
			{"name": "shotcut", "display_name": "Shotcut", "description": "Video editing", "category": "video", "entry_point": "cli-anything-shotcut", "skill_md": "shotcut/agent-harness/cli_anything/shotcut/skills/SKILL.md"},
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

func TestAgentRunCompactsHistoryAndEnforcesWindowLimit(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	llmStub := &compactionAwareLLM{}
	ag := New(Config{
		Name:             "assistant",
		Description:      "General helper",
		LLM:              llmStub,
		Memory:           mem,
		Skills:           skills.NewSkillsManager(""),
		Tools:            tools.NewRegistry(),
		MaxContextTokens: 700,
	})

	history := make([]prompt.Message, 0, 16)
	for i := 0; i < 8; i++ {
		history = append(history,
			prompt.Message{Role: "user", Content: fmt.Sprintf("old-user-%d %s", i, strings.Repeat("x", 120))},
			prompt.Message{Role: "assistant", Content: fmt.Sprintf("old-assistant-%d %s", i, strings.Repeat("y", 120))},
		)
	}
	ag.SetHistory(history)

	result, err := ag.Run(context.Background(), "latest request")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "final response" {
		t.Fatalf("unexpected response: %q", result)
	}

	if len(llmStub.messages) < 2 {
		t.Fatalf("expected summarizer call and main call, got %d batches", len(llmStub.messages))
	}

	mainBatch := llmStub.messages[len(llmStub.messages)-1]
	foundSummary := false
	foundOldest := false
	for _, msg := range mainBatch {
		if msg.Role == "system" && strings.Contains(msg.Content, "[Summary of previous conversation]") {
			foundSummary = true
		}
		if strings.Contains(msg.Content, "old-user-0") {
			foundOldest = true
		}
	}
	if !foundSummary {
		t.Fatalf("expected compacted history summary in main batch, got %#v", mainBatch)
	}
	if foundOldest {
		t.Fatalf("expected oldest history to be compacted away, got %#v", mainBatch)
	}
}

func TestBuildSystemPromptHotReloadsBootstrapFiles(t *testing.T) {
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
		Name:                   "assistant",
		Description:            "General helper",
		Memory:                 mem,
		Skills:                 skills.NewSkillsManager(""),
		Tools:                  tools.NewRegistry(),
		WorkingDir:             workingDir,
		BootstrapWatchInterval: 20 * time.Millisecond,
	})

	initialPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt(initial): %v", err)
	}
	if !strings.Contains(initialPrompt, "Complete the user's task safely") {
		t.Fatalf("expected initial bootstrap content, got %q", initialPrompt)
	}

	newAgents := "# AGENTS\n\n## Primary Agent\n- Goal: Prefer verifiable Go refactors.\n"
	if err := os.WriteFile(filepath.Join(workingDir, "AGENTS.md"), []byte(newAgents), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md): %v", err)
	}

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		reloadedPrompt, err := ag.buildSystemPrompt()
		if err != nil {
			t.Fatalf("buildSystemPrompt(reloaded): %v", err)
		}
		if strings.Contains(reloadedPrompt, "Prefer verifiable Go refactors.") {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}

	t.Fatal("expected bootstrap watcher to refresh AGENTS.md content")
}

func TestShowMemoryUsesMemoryService(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	if err := mem.Add(memory.MemoryEntry{
		Type:    memory.TypeFact,
		Content: "remember this",
	}); err != nil {
		t.Fatalf("memory add: %v", err)
	}

	ag := New(Config{
		Name:   "assistant",
		Memory: mem,
	})

	markdown, err := ag.ShowMemory()
	if err != nil {
		t.Fatalf("ShowMemory: %v", err)
	}
	if !strings.Contains(markdown, "# Memory") || !strings.Contains(markdown, "remember this") {
		t.Fatalf("expected markdown memory output, got %q", markdown)
	}
}

func TestAgentRunUsesExclusiveSlotToSerializeConcurrentExecutions(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	llmStub := newBlockingAgentLLM()
	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       tools.NewRegistry(),
	})

	results := make(chan string, 2)
	errs := make(chan error, 2)

	go func() {
		result, err := ag.Run(context.Background(), "first request")
		results <- result
		errs <- err
	}()

	select {
	case idx := <-llmStub.entered:
		if idx != 0 {
			t.Fatalf("expected first llm call index 0, got %d", idx)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first run to enter llm")
	}

	go func() {
		result, err := ag.Run(context.Background(), "second request")
		results <- result
		errs <- err
	}()

	select {
	case idx := <-llmStub.entered:
		t.Fatalf("second run entered llm before first released (idx=%d)", idx)
	case <-time.After(150 * time.Millisecond):
	}

	llmStub.Release(0)

	select {
	case idx := <-llmStub.entered:
		if idx != 1 {
			t.Fatalf("expected second llm call index 1 after release, got %d", idx)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second run to enter llm")
	}

	llmStub.Release(1)

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Run error: %v", err)
		}
		<-results
	}
}
