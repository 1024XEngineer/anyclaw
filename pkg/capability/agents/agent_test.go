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
	errs      []error
	index     int
	messages  [][]llm.Message
	toolDefs  [][]llm.ToolDefinition
}

type streamAgentTurn struct {
	response *llm.Response
	chunks   []string
}

type streamingAgentLLM struct {
	stubAgentLLM
	streamTurns []streamAgentTurn
	streamIndex int
}

func (s *streamingAgentLLM) StreamChatResponse(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) (*llm.Response, error) {
	s.messages = append(s.messages, append([]llm.Message(nil), messages...))
	s.toolDefs = append(s.toolDefs, append([]llm.ToolDefinition(nil), toolDefs...))
	if s.streamIndex >= len(s.streamTurns) {
		return s.Chat(ctx, messages, toolDefs)
	}
	turn := s.streamTurns[s.streamIndex]
	s.streamIndex++
	for _, chunk := range turn.chunks {
		if onChunk != nil {
			onChunk(chunk)
		}
	}
	if turn.response == nil {
		return &llm.Response{Content: strings.Join(turn.chunks, "")}, nil
	}
	return turn.response, nil
}

func (s *stubAgentLLM) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	s.messages = append(s.messages, append([]llm.Message(nil), messages...))
	s.toolDefs = append(s.toolDefs, append([]llm.ToolDefinition(nil), toolDefs...))
	if s.index < len(s.errs) && s.errs[s.index] != nil {
		err := s.errs[s.index]
		s.index++
		return nil, err
	}
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

func toolInfoListHas(list []tools.ToolInfo, name string) bool {
	for _, tool := range list {
		if tool.Name == name {
			return true
		}
	}
	return false
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
	if !strings.Contains(systemPrompt, "Codex-style coding and local execution agent") {
		t.Fatalf("expected Codex-style header in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "## Operating Mode") {
		t.Fatalf("expected operating mode section, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "## Identity") {
		t.Fatalf("expected identity section, got %q", systemPrompt)
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
	if !strings.Contains(systemPrompt, "read the codebase first") || !strings.Contains(systemPrompt, "Do not revert or overwrite user changes") {
		t.Fatalf("expected Codex-style coding workspace guidance, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "verify the requested deliverable with observable evidence") {
		t.Fatalf("expected verification guidance in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "## Final Response Style") || !strings.Contains(systemPrompt, "已处理") || !strings.Contains(systemPrompt, "已验证") || !strings.Contains(systemPrompt, "未确认/阻塞") {
		t.Fatalf("expected Codex-style final response guidance, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptInjectsMainSystemPrompt(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	reg := tools.NewRegistry()
	reg.RegisterTool("run_command", "Run a shell command", map[string]any{}, nil)

	ag := New(Config{
		Name:         "assistant",
		Description:  "General helper",
		SystemPrompt: "You are Claude Code style: verify outcomes before claiming success.",
		Personality:  "Be concise and action-oriented.",
		Memory:       mem,
		Skills:       skills.NewSkillsManager(""),
		Tools:        reg,
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	if !strings.Contains(systemPrompt, "## Identity") {
		t.Fatalf("expected identity section, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "Primary system prompt") || !strings.Contains(systemPrompt, "verify outcomes before claiming success") {
		t.Fatalf("expected main system prompt to be injected, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "Personality supplement") || !strings.Contains(systemPrompt, "Be concise and action-oriented.") {
		t.Fatalf("expected personality supplement to remain present, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptUsesOpenClawStyleSkillInstructions(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "weather")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillLocation := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillLocation, []byte("# Weather\n\nUse weather APIs carefully."), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{
  "name": "weather",
  "description": "Query weather forecasts and conditions.",
  "version": "1.0.0",
  "prompts": {"system": "DO NOT INLINE THIS PROMPT"}
}`), 0o644); err != nil {
		t.Fatalf("write skill.json: %v", err)
	}

	manager := skills.NewSkillsManager(root)
	if err := manager.Load(); err != nil {
		t.Fatalf("load skills: %v", err)
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Skills:      manager,
		Tools:       tools.NewRegistry(),
	})

	systemPrompt, err := ag.buildSystemPrompt()
	if err != nil {
		t.Fatalf("buildSystemPrompt: %v", err)
	}
	for _, want := range []string{
		"## Skills (mandatory)",
		"<available_skills>",
		"<name>weather</name>",
		"<description>Query weather forecasts and conditions.</description>",
		"<location>" + skillLocation + "</location>",
		"never read more than one skill up front",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected skill prompt to contain %q, got %q", want, systemPrompt)
		}
	}
	if strings.Contains(systemPrompt, "## Active Skills") || strings.Contains(systemPrompt, "DO NOT INLINE THIS PROMPT") {
		t.Fatalf("expected lazy skill loading prompt without inline system prompt, got %q", systemPrompt)
	}
}

func TestSelectToolInfosExposesReadForLazySkillInstructions(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "weather")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Weather"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{
  "name": "weather",
  "description": "Query weather forecasts and conditions.",
  "version": "1.0.0",
  "prompts": {"system": "Use weather."}
}`), 0o644); err != nil {
		t.Fatalf("write skill.json: %v", err)
	}

	manager := skills.NewSkillsManager(root)
	if err := manager.Load(); err != nil {
		t.Fatalf("load skills: %v", err)
	}
	registry := tools.NewRegistry()
	tools.RegisterOpenClawCompatTools(registry, tools.BuiltinOptions{})
	registry.RegisterTool("read_file", "Read a file", map[string]any{}, nil)
	ag := New(Config{Skills: manager, Tools: registry})

	selected := ag.selectToolInfos("hello")
	names := make(map[string]bool, len(selected))
	for _, tool := range selected {
		names[tool.Name] = true
	}
	if !names["read"] || !names["read_file"] {
		t.Fatalf("expected read tools for lazy skill loading, got %#v", selected)
	}
	if names["write"] || names["run_command"] {
		t.Fatalf("expected only read-oriented skill loading tools, got %#v", selected)
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
	if strings.Contains(systemPrompt, "## Operating Mode") {
		t.Fatalf("did not expect operating mode section without execution tools, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptIncludesCompactCapabilityPlan(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("market_search_artifacts", "Search marketplace artifacts", map[string]any{}, nil)
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, nil)

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Tools:       registry,
	})
	plan := CapabilityPlan{
		TaskClass:                CapabilityTaskSpecialized,
		Route:                    CapabilityRouteSearchMarket,
		Need:                     "code review",
		KindHint:                 "agent",
		ShouldExposeMarketSearch: true,
		Reason:                   "specialized capability is not available locally",
	}
	selected := ag.selectToolInfosWithPlan("review this pull request", &plan)
	systemPrompt, err := ag.buildSystemPromptForToolInfosWithPlan(selected, &plan)
	if err != nil {
		t.Fatalf("buildSystemPromptForToolInfosWithPlan: %v", err)
	}
	for _, want := range []string{"## Capability Planning", "Task class: specialized", "Planned route: search_market", "market_search_artifacts"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, systemPrompt)
		}
	}
	if strings.Contains(systemPrompt, "market_install_artifact:") {
		t.Fatalf("prompt should not expose install tool before confirmation, got %q", systemPrompt)
	}
}

func TestBuildSystemPromptDoesNotInjectMemoryByDefault(t *testing.T) {
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
	if strings.Contains(systemPrompt, "stable-fact") {
		t.Fatalf("expected durable memory to stay out of default prompt injection, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "conversation-11") || strings.Contains(systemPrompt, "conversation-00") {
		t.Fatalf("expected conversation memories to stay out of prompt injection, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "Prompt memory limited.") || strings.Contains(systemPrompt, "## Memory\n") {
		t.Fatalf("expected old prompt memory injection to stay removed, got %q", systemPrompt)
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
	registry.RegisterTool("computer_observe", "Observe computer", map[string]any{}, nil)
	registry.RegisterTool("computer_action", "Act on computer", map[string]any{}, nil)
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
	if !strings.Contains(got, "computer_observe") || !strings.Contains(got, "computer_action") || !strings.Contains(got, "desktop_open") || !strings.Contains(got, "desktop_list_windows") {
		t.Fatalf("expected desktop tools for Chinese open-app request, got %q", got)
	}
	if strings.Contains(got, "skill_app-controller") {
		t.Fatalf("expected skill tools to stay hidden unless explicitly requested, got %q", got)
	}
}

func TestSelectToolInfosTreatsGeneratedPageOpenAsDesktopRequest(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("read_file", "Read a file", map[string]any{}, nil)
	registry.RegisterTool("browser_navigate", "Navigate browser", map[string]any{}, nil)
	registry.RegisterTool("desktop_open", "Open a desktop target", map[string]any{}, nil)
	registry.RegisterTool("desktop_list_windows", "List desktop windows", map[string]any{}, nil)

	ag := New(Config{Tools: registry})

	actionable := ag.selectToolInfos("帮我打开你生成的页面")
	names := make([]string, 0, len(actionable))
	for _, tool := range actionable {
		names = append(names, tool.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "desktop_open") || !strings.Contains(got, "desktop_list_windows") {
		t.Fatalf("expected desktop tools for generated page open request, got %q", got)
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

func TestSelectToolInfosExposesOpenClawCompatibleCoreTools(t *testing.T) {
	registry := tools.NewRegistry()
	for _, name := range []string{"read", "write", "edit", "apply_patch", "exec", "process", "web_fetch", "image", "update_plan", "session_status"} {
		registry.RegisterTool(name, "OpenClaw-compatible core tool", map[string]any{}, nil)
	}

	ag := New(Config{Tools: registry})

	actionable := ag.selectToolInfos("read, edit, execute a command, fetch a URL, and analyze an image")
	names := make([]string, 0, len(actionable))
	for _, tool := range actionable {
		names = append(names, tool.Name)
	}
	got := strings.Join(names, ",")
	for _, name := range []string{"read", "write", "edit", "apply_patch", "exec", "process", "web_fetch", "image"} {
		if !strings.Contains(got, name) {
			t.Fatalf("expected OpenClaw-compatible tool %q to be exposed, got %q", name, got)
		}
	}

	planning := ag.selectToolInfos("show session status and update the plan steps")
	names = names[:0]
	for _, tool := range planning {
		names = append(names, tool.Name)
	}
	got = strings.Join(names, ",")
	for _, name := range []string{"update_plan", "session_status"} {
		if !strings.Contains(got, name) {
			t.Fatalf("expected OpenClaw-compatible tool %q to be exposed for planning/status, got %q", name, got)
		}
	}
}

func TestSelectToolInfosOnlyExposesRelevantCoreTools(t *testing.T) {
	registry := tools.NewRegistry()
	for _, name := range []string{"read", "write", "edit", "apply_patch", "exec", "process", "fetch_url", "web_fetch", "image", "image_analyze"} {
		registry.RegisterTool(name, "OpenClaw-compatible core tool", map[string]any{}, nil)
	}

	ag := New(Config{Tools: registry})

	selected := ag.selectToolInfos("fetch this URL: https://example.com/notes")
	names := make(map[string]bool, len(selected))
	for _, tool := range selected {
		names[tool.Name] = true
	}

	for _, name := range []string{"fetch_url", "web_fetch"} {
		if !names[name] {
			t.Fatalf("expected URL fetch tool %q to be exposed, got %#v", name, names)
		}
	}
	for _, name := range []string{"read", "write", "edit", "apply_patch", "exec", "process", "image", "image_analyze"} {
		if names[name] {
			t.Fatalf("expected unrelated core tool %q to stay hidden, got %#v", name, names)
		}
	}
}

func TestSelectToolInfosUsesCodexCodeToolsForChineseAppBuild(t *testing.T) {
	registry := tools.NewRegistry()
	for _, name := range []string{
		"read_file", "write_file", "list_directory", "search_files", "run_command",
		"desktop_open", "desktop_list_windows", "browser_navigate", "computer_observe", "computer_action", "skill_app-controller",
	} {
		registry.RegisterTool(name, "tool "+name, map[string]any{}, nil)
	}

	ag := New(Config{Tools: registry})

	selected := ag.selectToolInfos("帮我生成一个贪吃蛇小游戏")
	names := make(map[string]bool, len(selected))
	for _, tool := range selected {
		names[tool.Name] = true
	}

	for _, name := range []string{"read_file", "write_file", "list_directory", "search_files", "run_command"} {
		if !names[name] {
			t.Fatalf("expected Codex code tool %q for app build request, got %#v", name, names)
		}
	}
	for _, name := range []string{"desktop_open", "desktop_list_windows", "browser_navigate", "computer_observe", "computer_action", "skill_app-controller"} {
		if names[name] {
			t.Fatalf("expected unrelated tool %q to stay hidden for app build request, got %#v", name, names)
		}
	}
}

func TestSelectToolInfosWithPlanExposesOnlyMarketSearchForSpecializedGap(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("market_search_artifacts", "Search marketplace artifacts", map[string]any{}, nil)
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, nil)
	registry.RegisterTool("market_bind_artifact", "Bind marketplace artifact", map[string]any{}, nil)
	registry.RegisterTool("read_file", "Read file", map[string]any{}, nil)

	ag := New(Config{Tools: registry})
	plan := CapabilityPlanner{}.Plan(CapabilityPlannerInput{
		UserInput: "review this pull request for security and maintainability",
	})

	selected := ag.selectToolInfosWithPlan("review this pull request for security and maintainability", &plan)
	names := make(map[string]bool, len(selected))
	for _, tool := range selected {
		names[tool.Name] = true
	}

	if !names["market_search_artifacts"] {
		t.Fatalf("expected market search to be exposed, got %#v", names)
	}
	if names["market_install_artifact"] || names["market_bind_artifact"] {
		t.Fatalf("install/bind must stay hidden before explicit user confirmation, got %#v", names)
	}
}

func TestSelectToolInfosExposesMarketInstallOnlyAfterConfirmation(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("market_search_artifacts", "Search marketplace artifacts", map[string]any{}, nil)
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, nil)
	registry.RegisterTool("market_bind_artifact", "Bind marketplace artifact", map[string]any{}, nil)

	ag := New(Config{Tools: registry})
	ag.SetHistory([]prompt.Message{
		{Role: "assistant", Content: "Cloud candidate: Code Reviewer (cloud.agent.code-reviewer, medium risk, verified)."},
	})

	selected := ag.selectToolInfos("确认安装这个 code reviewer")
	names := make(map[string]bool, len(selected))
	for _, tool := range selected {
		names[tool.Name] = true
	}
	if !names["market_install_artifact"] {
		t.Fatalf("expected install tool after explicit confirmation, got %#v", names)
	}
	if names["market_bind_artifact"] {
		t.Fatalf("did not expect bind tool for install-only confirmation, got %#v", names)
	}
}

func TestSelectToolInfosDoesNotTreatVersionAsMarketArtifactConfirmation(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, nil)

	ag := New(Config{Tools: registry})
	ag.SetHistory([]prompt.Message{
		{Role: "assistant", Content: "Version 1.0.0 is available."},
	})
	selected := ag.selectToolInfos("yes install it")
	if toolInfoListHas(selected, "market_install_artifact") {
		t.Fatalf("did not expect install tool for version-only context, got %#v", selected)
	}
}

func TestSelectToolInfosExposesMarketBindAfterBindingConfirmation(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, nil)
	registry.RegisterTool("market_bind_artifact", "Bind marketplace artifact", map[string]any{}, nil)

	ag := New(Config{Tools: registry})
	ag.SetHistory([]prompt.Message{
		{Role: "assistant", Content: "Installed cloud.skill.release-notes successfully. It can be bound to main_agent."},
	})

	selected := ag.selectToolInfos("绑定到 main agent 并启用")
	names := make(map[string]bool, len(selected))
	for _, tool := range selected {
		names[tool.Name] = true
	}
	if !names["market_bind_artifact"] {
		t.Fatalf("expected bind tool after binding confirmation, got %#v", names)
	}
}

func TestBuildSystemPromptIncludesMarketplaceConfirmationGuidance(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, nil)

	ag := New(Config{Tools: registry})
	ag.SetHistory([]prompt.Message{
		{Role: "assistant", Content: "Cloud candidate: cloud.agent.code-reviewer."},
	})
	selected := ag.selectToolInfos("yes install it")
	systemPrompt, err := ag.buildSystemPromptForToolInfosWithPlan(selected, &CapabilityPlan{
		TaskClass: CapabilityTaskSpecialized,
		Route:     CapabilityRouteMainAgent,
		Need:      "code review",
	})
	if err != nil {
		t.Fatalf("buildSystemPromptForToolInfosWithPlan: %v", err)
	}
	if !strings.Contains(systemPrompt, "follow-up confirmation") || !strings.Contains(systemPrompt, "requires_confirmation") {
		t.Fatalf("expected marketplace confirmation guidance, got %q", systemPrompt)
	}
}

func TestSelectToolInfosWithPlanExposesLocalRoutes(t *testing.T) {
	registry := tools.NewRegistry()
	for _, name := range []string{"skill_release-notes", "delegate_task", "intent_route", "intent_list_capabilities", "clihub_exec", "clihub_catalog"} {
		registry.RegisterTool(name, "tool "+name, map[string]any{}, nil)
	}
	ag := New(Config{Tools: registry})

	skillPlan := &CapabilityPlan{
		Route:        CapabilityRouteUseSkill,
		TaskClass:    CapabilityTaskSpecialized,
		LocalMatches: []CapabilityMatch{{Kind: "skill", Name: "release-notes", Score: 0.9}},
	}
	if selected := ag.selectToolInfosWithPlan("write release notes", skillPlan); !toolInfoListHas(selected, "skill_release-notes") {
		t.Fatalf("expected matching skill tool, got %#v", selected)
	}

	agentPlan := &CapabilityPlan{Route: CapabilityRouteDelegateAgent, TaskClass: CapabilityTaskSpecialized}
	if selected := ag.selectToolInfosWithPlan("review this pull request", agentPlan); !toolInfoListHas(selected, "delegate_task") {
		t.Fatalf("expected delegate_task, got %#v", selected)
	}

	cliPlan := &CapabilityPlan{Route: CapabilityRouteUseCLI, TaskClass: CapabilityTaskLocalExecution}
	selected := ag.selectToolInfosWithPlan("render the timeline", cliPlan)
	for _, name := range []string{"intent_route", "intent_list_capabilities", "clihub_exec"} {
		if !toolInfoListHas(selected, name) {
			t.Fatalf("expected CLIHub tool %q, got %#v", name, selected)
		}
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

func TestAgentRunPresearchesMarketplaceForSpecializedGap(t *testing.T) {
	registry := tools.NewRegistry()
	var searched bool
	registry.RegisterTool("market_search_artifacts", "Search marketplace artifacts", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		searched = true
		if input["query"] != "code review" || input["kind"] != "agent" {
			t.Fatalf("unexpected market search input: %#v", input)
		}
		return `{
			"route":{"action":"install_from_market","reason":"cloud candidate available"},
			"local_count":0,
			"cloud_count":1,
			"cloud":[{"artifact_id":"cloud.agent.code-reviewer","kind":"agent","name":"Code Reviewer","risk_level":"medium","trust_level":"verified"}]
		}`, nil
	})
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		t.Fatal("market install must not run during presearch")
		return "", nil
	})
	registry.RegisterTool("market_bind_artifact", "Bind marketplace artifact", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		t.Fatal("market bind must not run during presearch")
		return "", nil
	})

	llmStub := &stubAgentLLM{responses: []*llm.Response{{Content: "我找到了一个可选的 code review agent，需要你确认后再安装。"}}}
	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Tools:       registry,
	})

	if _, err := ag.Run(context.Background(), "review this pull request for security and maintainability"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !searched {
		t.Fatal("expected marketplace search to run before model turn")
	}
	if len(llmStub.messages) == 0 {
		t.Fatal("expected model messages")
	}
	joined := ""
	for _, msg := range llmStub.messages[0] {
		joined += "\n" + msg.Content
	}
	if !strings.Contains(joined, "## Capability Planning Search") || !strings.Contains(joined, "cloud.agent.code-reviewer") {
		t.Fatalf("expected compact market search context, got %q", joined)
	}
	for _, def := range llmStub.toolDefs[0] {
		if def.Function.Name == "market_install_artifact" || def.Function.Name == "market_bind_artifact" {
			t.Fatalf("install/bind should not be exposed before user confirmation, got %#v", llmStub.toolDefs[0])
		}
	}
	activities := ag.GetLastToolActivities()
	if len(activities) != 1 || activities[0].ToolName != "market_search_artifacts" || activities[0].Error != "" {
		t.Fatalf("expected one successful market search activity, got %#v", activities)
	}
}

func TestAgentRunExposesInstallToolOnMarketplaceConfirmationTurn(t *testing.T) {
	registry := tools.NewRegistry()
	registry.RegisterTool("market_install_artifact", "Install marketplace artifact", map[string]any{}, nil)
	registry.RegisterTool("market_bind_artifact", "Bind marketplace artifact", map[string]any{}, nil)

	llmStub := &stubAgentLLM{responses: []*llm.Response{{Content: "我会安装这个 artifact。"}}}
	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Tools:       registry,
	})
	ag.SetHistory([]prompt.Message{
		{Role: "user", Content: "review this pull request"},
		{Role: "assistant", Content: "Cloud candidate: Code Reviewer (cloud.agent.code-reviewer). Install it?"},
	})

	if _, err := ag.Run(context.Background(), "yes, install it"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(llmStub.toolDefs) == 0 {
		t.Fatal("expected tool definitions")
	}
	names := map[string]bool{}
	for _, def := range llmStub.toolDefs[0] {
		names[def.Function.Name] = true
	}
	if !names["market_install_artifact"] {
		t.Fatalf("expected install tool on confirmation turn, got %#v", names)
	}
	if names["market_bind_artifact"] {
		t.Fatalf("did not expect bind tool on install confirmation turn, got %#v", names)
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
	if !strings.Contains(systemPrompt, "### AGENTS.md") {
		t.Fatalf("expected AGENTS.md to be injected, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "### MEMORY.md") {
		t.Fatalf("expected old MEMORY.md prompt injection to stay removed, got %q", systemPrompt)
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
	registry.RegisterTool("claw_bridge_context", "Read claw bridge context", map[string]any{}, nil)

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
		if msg.Role == "user" && strings.Contains(msg.Content, "Structured tool observations above are evidence about the current world state") && strings.Contains(msg.Content, "Before claiming completion, verify the outcome") {
			foundFollowup = true
			break
		}
	}
	if !foundFollowup {
		t.Fatalf("expected observation/verification follow-up prompt, got %#v", llmStub.messages[1])
	}
}

func TestAgentRunCompletesWithToolResultWhenFinalLLMTimesOut(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("desktop_open", "Open desktop target", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "opened file", nil
	})

	llmStub := &stubAgentLLM{
		responses: []*llm.Response{
			{
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tool-open",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "desktop_open",
							Arguments: `{"target":"snake_game.html","kind":"file"}`,
						},
					},
				},
			},
		},
		errs: []error{nil, context.DeadlineExceeded},
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	result, err := ag.Run(context.Background(), "帮我打开snake_game.html")
	if err != nil {
		t.Fatalf("Run should complete from successful tool result: %v", err)
	}
	for _, want := range []string{"已处理", "已打开文件 snake_game.html", "模型生成补充说明失败"} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected fallback result to contain %q, got %q", want, result)
		}
	}
	if len(llmStub.messages) != 2 {
		t.Fatalf("expected initial tool turn plus failed final turn, got %d", len(llmStub.messages))
	}
	activities := ag.GetLastToolActivities()
	if len(activities) != 1 || activities[0].ToolName != "desktop_open" || activities[0].Error != "" {
		t.Fatalf("expected successful desktop_open activity, got %#v", activities)
	}
}

func TestAgentRunDoesNotCompleteFromReadOnlyToolWhenFinalLLMTimesOut(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("read_file", "Read a file", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "<html></html>", nil
	})

	llmStub := &stubAgentLLM{
		responses: []*llm.Response{
			{
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tool-read",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "read_file",
							Arguments: `{"path":"anyclaw_webpage.html"}`,
						},
					},
				},
			},
		},
		errs: []error{nil, context.DeadlineExceeded},
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	result, err := ag.Run(context.Background(), "帮我打开你生成的页面")
	if err == nil {
		t.Fatalf("expected read-only tool fallback to keep the task failed, got result %q", result)
	}
	if strings.Contains(result, "已处理") || strings.Contains(result, "未确认/阻塞：\n- 无") {
		t.Fatalf("did not expect completion-style response from read-only fallback, got %q", result)
	}
	activities := ag.GetLastToolActivities()
	if len(activities) != 1 || activities[0].ToolName != "read_file" || activities[0].Error != "" {
		t.Fatalf("expected recorded read_file activity, got %#v", activities)
	}
}

func TestAgentRunDoesNotCompleteFromRunCommandWhenFinalLLMTimesOut(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "File does not exist", nil
	})

	llmStub := &stubAgentLLM{
		responses: []*llm.Response{
			{
				ToolCalls: []llm.ToolCall{
					{
						ID:   "tool-run",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "run_command",
							Arguments: `{"command":"start missing.html"}`,
						},
					},
				},
			},
		},
		errs: []error{nil, context.DeadlineExceeded},
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	result, err := ag.Run(context.Background(), "帮我打开你生成的页面")
	if err == nil {
		t.Fatalf("expected run_command fallback to keep the task failed, got result %q", result)
	}
	if strings.Contains(result, "已处理") || strings.Contains(result, "未确认/阻塞：\n- 无") {
		t.Fatalf("did not expect completion-style response from run_command fallback, got %q", result)
	}
}

func TestAgentRunSummarizesPageOpenToolFallback(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		switch strings.TrimSpace(fmt.Sprint(input["command"])) {
		case "Test-Path 'D:\\anyclaw\\anyclaw\\workflows\\default\\anyclaw_webpage.html'":
			return "True", nil
		case "Invoke-Item 'D:\\anyclaw\\anyclaw\\workflows\\default\\anyclaw_webpage.html'":
			return "", nil
		case "Start-Process 'msedge.exe' -ArgumentList 'D:\\anyclaw\\anyclaw\\workflows\\default\\anyclaw_webpage.html'":
			return "", nil
		case "Get-Process msedge":
			return "Handles  NPM(K)    PM(K)      WS(K)     CPU(s)     Id  SI ProcessName\n  1000      45   120000     180000       1.23  12345   1 msedge", nil
		default:
			return "", fmt.Errorf("unexpected command")
		}
	})
	registry.RegisterTool("computer_observe", "Observe computer", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return `{"protocol":"anyclaw.computer.codex.v1","environment":"desktop","screenshot_path":"D:\\observe.png","windows":[{"title":"anyclaw_webpage.html - Microsoft Edge","process_name":"msedge"}]}`, nil
	})

	llmStub := &stubAgentLLM{
		responses: []*llm.Response{
			{ToolCalls: []llm.ToolCall{
				{ID: "tool-test", Type: "function", Function: llm.FunctionCall{Name: "run_command", Arguments: `{"command":"Test-Path 'D:\\anyclaw\\anyclaw\\workflows\\default\\anyclaw_webpage.html'"}`}},
				{ID: "tool-open", Type: "function", Function: llm.FunctionCall{Name: "run_command", Arguments: `{"command":"Invoke-Item 'D:\\anyclaw\\anyclaw\\workflows\\default\\anyclaw_webpage.html'"}`}},
				{ID: "tool-edge", Type: "function", Function: llm.FunctionCall{Name: "run_command", Arguments: `{"command":"Start-Process 'msedge.exe' -ArgumentList 'D:\\anyclaw\\anyclaw\\workflows\\default\\anyclaw_webpage.html'"}`}},
				{ID: "tool-proc", Type: "function", Function: llm.FunctionCall{Name: "run_command", Arguments: `{"command":"Get-Process msedge"}`}},
				{ID: "tool-observe", Type: "function", Function: llm.FunctionCall{Name: "computer_observe", Arguments: `{}`}},
			}},
		},
		errs: []error{nil, context.DeadlineExceeded},
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	result, err := ag.Run(context.Background(), "帮我启动你创建的页面")
	if err != nil {
		t.Fatalf("Run should complete from page open tool evidence: %v", err)
	}
	for _, want := range []string{
		"已启动浏览器打开 D:\\anyclaw\\anyclaw\\workflows\\default\\anyclaw_webpage.html",
		"已确认目标文件存在",
		"已检测到浏览器进程正在运行",
		"桌面窗口列表中已出现浏览器窗口",
		"未确认/阻塞：\n- 无",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected fallback result to contain %q, got %q", want, result)
		}
	}
	for _, noisy := range []string{"run_command completed", "computer_observe completed", "已完成 `run_command`"} {
		if strings.Contains(result, noisy) {
			t.Fatalf("did not expect raw tool-log phrasing %q in result: %q", noisy, result)
		}
	}
}

func TestAgentRunDoesNotExposeMaxToolCallsMarker(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return "still running", nil
	})
	toolTurn := &llm.Response{
		ToolCalls: []llm.ToolCall{
			{
				ID:   "tool-repeat",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      "run_command",
					Arguments: `{"command":"echo hi"}`,
				},
			},
		},
	}
	llmStub := &stubAgentLLM{responses: []*llm.Response{}}
	for i := 0; i < 12; i++ {
		llmStub.responses = append(llmStub.responses, toolTurn)
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})
	ag.maxToolCalls = 1

	result, err := ag.Run(context.Background(), "repeat tool calls")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(result, "[Max tool calls reached]") {
		t.Fatalf("did not expect internal marker in result: %q", result)
	}
	if !strings.Contains(result, "工具调用次数达到上限") {
		t.Fatalf("expected tool limit message, got %q", result)
	}
	if strings.Contains(result, "已完成 `run_command`") {
		t.Fatalf("did not expect run_command to be reported as completed fallback, got %q", result)
	}
}

func TestAgentRunUsesCompactStructuredObservationForLargeToolResult(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	largeOutput := strings.Repeat("large-output-", 900)
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return largeOutput, nil
	})

	llmStub := &stubAgentLLM{responses: []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-large",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"produce large output"}`,
					},
				},
			},
		},
		{Content: "done"},
	}}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	if _, err := ag.Run(context.Background(), "run command with large output"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(llmStub.messages) < 2 {
		t.Fatalf("expected second llm turn after tool call, got %d", len(llmStub.messages))
	}
	var toolMessage *llm.Message
	for i := range llmStub.messages[1] {
		msg := &llmStub.messages[1][i]
		if msg.Role == "tool" {
			toolMessage = msg
			break
		}
	}
	if toolMessage == nil {
		t.Fatalf("expected structured tool message in second turn, got %#v", llmStub.messages[1])
	}
	if !strings.Contains(toolMessage.Content, `"type":"codex_tool_observation"`) {
		t.Fatalf("expected codex observation JSON, got %q", toolMessage.Content)
	}
	if !strings.Contains(toolMessage.Content, `"output_truncated":true`) {
		t.Fatalf("expected truncated observation, got %q", toolMessage.Content)
	}
	if len(toolMessage.Content) >= len(largeOutput) {
		t.Fatalf("expected compact tool message shorter than raw output, got %d >= %d", len(toolMessage.Content), len(largeOutput))
	}
	if !strings.Contains(toolMessage.Content, fmt.Sprintf(`"output_chars":%d`, len(largeOutput))) {
		t.Fatalf("expected original output length metadata, got %q", toolMessage.Content)
	}
}

func TestResumeAfterApprovalUsesCodexObservations(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	legacyOutput := strings.Repeat("legacy-output-", 900)
	pendingOutput := strings.Repeat("pending-output-", 900)
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return pendingOutput, nil
	})

	llmStub := &stubAgentLLM{responses: []*llm.Response{{Content: "resumed done"}}}
	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	resume := ApprovalResumeState{
		Messages: []llm.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "run two commands"},
		},
		AssistantMessage: llm.Message{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-old",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"old"}`,
					},
				},
				{
					ID:   "tool-pending",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"pending"}`,
					},
				},
			},
		},
		Observations: []ToolObservation{
			newToolObservation(ToolCall{
				ID:   "tool-old",
				Name: "run_command",
				Args: map[string]any{"command": "old"},
			}, legacyOutput, nil),
		},
		PendingTool: ToolCall{ID: "tool-pending", Name: "run_command", Args: map[string]any{"command": "pending"}},
	}

	if _, err := ag.ResumeAfterApproval(context.Background(), resume); err != nil {
		t.Fatalf("ResumeAfterApproval: %v", err)
	}
	if len(llmStub.messages) != 1 {
		t.Fatalf("expected one resumed llm call, got %d", len(llmStub.messages))
	}
	for _, msg := range llmStub.messages[0] {
		if msg.Role != "tool" {
			continue
		}
		if !strings.Contains(msg.Content, `"type":"codex_tool_observation"`) {
			t.Fatalf("expected compact observation, got %q", msg.Content)
		}
		if len(msg.Content) >= len(legacyOutput) {
			t.Fatalf("expected codex tool output to be compacted, got %d >= %d", len(msg.Content), len(legacyOutput))
		}
	}
}

func TestAgentRunSummarizesOlderToolObservationsAcrossTurns(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		return strings.Repeat("turn-output-", 500), nil
	})

	toolTurn := func(id string) *llm.Response {
		return &llm.Response{
			ToolCalls: []llm.ToolCall{
				{
					ID:   id,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: fmt.Sprintf(`{"command":"cmd-%s"}`, id),
					},
				},
			},
		}
	}
	llmStub := &stubAgentLLM{responses: []*llm.Response{
		toolTurn("tool-1"),
		toolTurn("tool-2"),
		toolTurn("tool-3"),
		toolTurn("tool-4"),
		toolTurn("tool-5"),
		toolTurn("tool-6"),
		toolTurn("tool-7"),
		{Content: "done"},
	}}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	if _, err := ag.Run(context.Background(), "run many tool turns"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	lastBatch := llmStub.messages[len(llmStub.messages)-1]
	var summarizedOld bool
	var recentWithOutput int
	for _, msg := range lastBatch {
		if msg.Role != "tool" {
			continue
		}
		if strings.Contains(msg.Content, `"output":"`) {
			recentWithOutput++
		}
		if strings.Contains(msg.Content, `"tool_call_id":"tool-1"`) && !strings.Contains(msg.Content, `"output":"`) && strings.Contains(msg.Content, `"output_truncated":true`) {
			summarizedOld = true
		}
	}
	if !summarizedOld {
		t.Fatalf("expected oldest tool observation to be summary-only, got %#v", lastBatch)
	}
	if recentWithOutput > codexLoopRecentToolOutputs {
		t.Fatalf("expected at most %d recent outputs, got %d", codexLoopRecentToolOutputs, recentWithOutput)
	}
}

func TestAgentRunStreamExecutesToolLoop(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	called := 0
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		called++
		return "stream build succeeded", nil
	})

	llmStub := &stubAgentLLM{responses: []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "tool-stream-1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "run_command",
						Arguments: `{"command":"go test ./..."}`,
					},
				},
			},
		},
		{Content: "stream final response"},
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

	var streamed strings.Builder
	err := ag.RunStream(context.Background(), "run tests in stream mode", func(chunk string) {
		streamed.WriteString(chunk)
	})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected stream path to execute one tool call, got %d", called)
	}
	if streamed.String() != "stream final response" {
		t.Fatalf("unexpected streamed response %q", streamed.String())
	}
}

func TestAgentRunStreamEmitsModelChunksDuringToolLoop(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	called := 0
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		called++
		return "stream build succeeded", nil
	})

	toolTurn := &llm.Response{
		ToolCalls: []llm.ToolCall{
			{
				ID:   "tool-stream-1",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      "run_command",
					Arguments: `{"command":"go test ./..."}`,
				},
			},
		},
	}
	finalTurn := &llm.Response{Content: "stream final response"}
	llmStub := &streamingAgentLLM{
		stubAgentLLM: stubAgentLLM{responses: []*llm.Response{
			toolTurn,
			finalTurn,
		}},
		streamTurns: []streamAgentTurn{
			{response: toolTurn},
			{response: finalTurn, chunks: []string{"stream final ", "response"}},
		},
	}

	ag := New(Config{
		Name:        "assistant",
		Description: "General helper",
		Personality: "Operate like an execution-focused local app agent.",
		LLM:         llmStub,
		Memory:      mem,
		Skills:      skills.NewSkillsManager(""),
		Tools:       registry,
	})

	var chunks []string
	err := ag.RunStream(context.Background(), "run tests in stream mode", func(chunk string) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected stream path to execute one tool call, got %d", called)
	}
	if got := strings.Join(chunks, ""); got != "stream final response" {
		t.Fatalf("unexpected streamed response %q", got)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected streamed model chunks to be forwarded individually, got %#v", chunks)
	}
}

func TestAgentRunBlocksRepeatedIdenticalToolLoop(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	registry := tools.NewRegistry()
	called := 0
	registry.RegisterTool("run_command", "Run a shell command", map[string]any{}, func(ctx context.Context, input map[string]any) (string, error) {
		called++
		return "same result", nil
	})

	repeatedCall := &llm.Response{
		ToolCalls: []llm.ToolCall{
			{
				ID:   "tool-loop",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      "run_command",
					Arguments: `{"command":"echo stuck"}`,
				},
			},
		},
	}
	llmStub := &stubAgentLLM{responses: []*llm.Response{
		repeatedCall,
		repeatedCall,
		repeatedCall,
		repeatedCall,
		{Content: "should not reach final"},
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

	_, err := ag.Run(context.Background(), "repeat until stuck")
	if err == nil || !strings.Contains(err.Error(), "tool loop detected") {
		t.Fatalf("expected tool loop detected error, got %v", err)
	}
	activities := ag.GetLastToolActivities()
	if len(activities) != 3 {
		t.Fatalf("expected loop detector to block before fourth identical tool attempt, got %d activities and %d handler calls", len(activities), called)
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

func TestAgentRunSkipsDeletedBootstrapRitual(t *testing.T) {
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
		t.Fatalf("Run: %v", err)
	}
	if answer != "normal task response" {
		t.Fatalf("expected normal llm response after deleted bootstrap ritual, got %q", answer)
	}
	if len(llmStub.messages) != 1 {
		t.Fatalf("expected llm to be called directly, got %d calls", len(llmStub.messages))
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
		MaxContextTokens: 1100,
	})

	history := make([]prompt.Message, 0, 16)
	for i := 0; i < 8; i++ {
		history = append(history,
			prompt.Message{Role: "user", Content: fmt.Sprintf("old-user-%d %s", i, strings.Repeat("x", 240))},
			prompt.Message{Role: "assistant", Content: fmt.Sprintf("old-assistant-%d %s", i, strings.Repeat("y", 240))},
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

func TestAgentRunAccountsForToolDefinitionsInContextBudget(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}

	registry := tools.NewRegistry()
	largeSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": strings.Repeat("large schema ", 1200),
			},
		},
	}
	for _, name := range []string{"read_file", "write_file", "list_directory", "search_files", "run_command"} {
		registry.RegisterTool(name, strings.Repeat("description ", 80), largeSchema, nil)
	}

	llmStub := &stubAgentLLM{responses: []*llm.Response{{Content: "ok"}}}
	ag := New(Config{
		Name:             "assistant",
		Description:      "General helper",
		LLM:              llmStub,
		Memory:           mem,
		Skills:           skills.NewSkillsManager(""),
		Tools:            registry,
		MaxContextTokens: 3600,
	})

	result, err := ag.Run(context.Background(), "帮我生成一个贪吃蛇小游戏")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "ok" {
		t.Fatalf("unexpected result: %q", result)
	}
	if len(llmStub.toolDefs) != 1 {
		t.Fatalf("expected one main model call, got %d", len(llmStub.toolDefs))
	}
	totalBytes := 0
	for _, def := range llmStub.toolDefs[0] {
		data, _ := json.Marshal(def)
		totalBytes += len(data)
	}
	if totalBytes > 9000 {
		t.Fatalf("expected compact tool definitions, got %d bytes across %#v", totalBytes, llmStub.toolDefs[0])
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
