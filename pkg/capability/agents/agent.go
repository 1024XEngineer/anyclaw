package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/prompt"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/clawbridge"
	"github.com/1024XEngineer/anyclaw/pkg/clihub"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	ctxpkg "github.com/1024XEngineer/anyclaw/pkg/runtime/context/store"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/workspace"
)

type LLMCaller interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error)
	StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error
	Name() string
}

type Agent struct {
	config             Config
	llm                LLMCaller
	memory             memory.MemoryBackend
	skills             *skills.SkillsManager
	tools              *tools.Registry
	workDir            string
	workingDir         string
	history            []prompt.Message
	maxToolCalls       int
	observer           Observer
	observerMu         sync.RWMutex
	lastToolActivities []ToolActivity
	intentPreprocessor *IntentPreprocessor
	preferenceLearner  *PreferenceLearner
	contextRuntime     *contextRuntime
}

type Config struct {
	Name        string
	Description string
	Personality string
	IsSubAgent  bool
	LLM         LLMCaller
	Memory      memory.MemoryBackend
	Skills      *skills.SkillsManager
	Tools       *tools.Registry
	WorkDir     string
	WorkingDir  string
	CLIHubRoot  string

	MaxContextTokens       int
	ContextSafetyMargin    int
	BootstrapWatchInterval time.Duration
	ContextEngine          ctxpkg.ContextEngine
}

var (
	codeBlockRegex = regexp.MustCompile("(?s)```(?:json)?[\\s]*(.+?)[\\s]*```")
	writeFileRegex = regexp.MustCompile("write_file\\s+path\\s*=\\s*\"([^\"]+)\"\\s+content\\s*=\\s*\"([\\s\\S]*?)\"")
	readFileRegex  = regexp.MustCompile("read_file\\s+path\\s*=\\s*\"([^\"]+)\"")
	listDirRegex   = regexp.MustCompile("list_directory\\s+path\\s*=\\s*\"([^\"]+)\"")
	searchRegex    = regexp.MustCompile("search_files\\s+path\\s*=\\s*\"([^\"]+)\"\\s+pattern\\s*=\\s*\"([^\"]+)\"")
	runCmdRegex    = regexp.MustCompile("run_command\\s+command\\s*=\\s*\"([^\"]+)\"")
)

const (
	promptMemoryMaxChars      = 4000
	promptMemoryMaxEntries    = 8
	promptMemoryEntryMaxChars = 600
)

func New(cfg Config) *Agent {
	agent := &Agent{
		config:       cfg,
		llm:          cfg.LLM,
		memory:       cfg.Memory,
		skills:       cfg.Skills,
		tools:        cfg.Tools,
		workDir:      cfg.WorkDir,
		workingDir:   cfg.WorkingDir,
		history:      []prompt.Message{},
		maxToolCalls: 10,
	}
	agent.contextRuntime = newContextRuntime(cfg)

	if cfg.CLIHubRoot != "" {
		agent.intentPreprocessor, _ = NewIntentPreprocessor(cfg.CLIHubRoot, nil)
	}

	if cfg.WorkingDir != "" {
		agent.preferenceLearner = NewPreferenceLearner(cfg.WorkingDir)
	}

	return agent
}

func (a *Agent) EnableIntentPreprocessing(root string, execFunc func([]string, string) (string, error)) error {
	ip, err := NewIntentPreprocessor(root, execFunc)
	if err != nil {
		return err
	}
	a.intentPreprocessor = ip
	return nil
}

func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	a.resetToolActivities()
	exec, err := a.acquireContextExecution(ctx, userInput)
	if err != nil {
		return "", err
	}
	defer a.releaseContextExecution(exec)

	if bootstrapResult, handled, err := a.handleBootstrapRitual(ctx, userInput); handled {
		return bootstrapResult, err
	}
	if execResult, handled, err := a.tryAutoRouteCLIHubIntent(ctx, userInput); handled {
		return execResult, err
	}
	selectedTools := a.selectToolInfos(userInput)
	a.appendHistoryMessage(ctx, "user", userInput)
	systemPrompt, err := a.prepareSystemPrompt(ctx, selectedTools)
	if err != nil {
		return "", err
	}
	messages := a.buildMessages(systemPrompt)
	toolDefs := buildToolDefinitionsFromInfos(selectedTools)

	a.heartbeatContextExecution(exec)
	response, err := a.chatWithTools(ctx, messages, toolDefs)
	if err != nil {
		return "", err
	}

	a.appendHistoryMessage(ctx, "assistant", response)

	a.memory.Add(memory.MemoryEntry{Type: "conversation", Role: "user", Content: userInput})
	a.memory.Add(memory.MemoryEntry{Type: "conversation", Role: "assistant", Content: response})

	if a.preferenceLearner != nil {
		if prefResponse, learned := a.preferenceLearner.Learn(userInput, response); learned {
			response = prefResponse + "\n\n" + response
		}
	}

	return response, nil
}

func (a *Agent) RunStream(ctx context.Context, userInput string, onChunk func(string)) error {
	a.resetToolActivities()
	exec, err := a.acquireContextExecution(ctx, userInput)
	if err != nil {
		return err
	}
	defer a.releaseContextExecution(exec)

	if bootstrapResult, handled, err := a.handleBootstrapRitual(ctx, userInput); handled {
		if err != nil {
			return err
		}
		if onChunk != nil {
			onChunk(bootstrapResult)
		}
		return nil
	}
	if execResult, handled, err := a.tryAutoRouteCLIHubIntent(ctx, userInput); handled {
		if err != nil {
			return err
		}
		if onChunk != nil {
			onChunk(execResult)
		}
		return nil
	}

	selectedTools := a.selectToolInfos(userInput)
	a.appendHistoryMessage(ctx, "user", userInput)
	systemPrompt, err := a.prepareSystemPrompt(ctx, selectedTools)
	if err != nil {
		return err
	}
	messages := a.buildMessages(systemPrompt)
	toolDefs := buildToolDefinitionsFromInfos(selectedTools)

	a.heartbeatContextExecution(exec)
	err = a.llm.StreamChat(ctx, messages, toolDefs, func(chunk string) {
		onChunk(chunk)
	})
	if err != nil {
		return err
	}

	return nil
}

func (a *Agent) handleBootstrapRitual(ctx context.Context, userInput string) (string, bool, error) {
	if strings.TrimSpace(a.workingDir) == "" {
		return "", false, nil
	}
	result, err := workspace.AdvanceBootstrapRitual(a.workingDir, userInput, workspace.BootstrapRitualOptions{
		AgentName:        a.config.Name,
		AgentDescription: a.config.Description,
	})
	if err != nil {
		return "", true, err
	}
	if result == nil || !result.Active {
		return "", false, nil
	}
	a.appendHistoryMessage(ctx, "user", userInput)
	a.appendHistoryMessage(ctx, "assistant", result.Response)
	a.recordConversation(userInput, result.Response)
	return result.Response, true, nil
}

func (a *Agent) chatWithTools(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (string, error) {
	for toolCalls := 0; ; toolCalls++ {
		resp, err := a.llm.Chat(ctx, messages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			if result, handled, err := a.executeProtocolResponse(ctx, resp); handled {
				if err != nil {
					return "", err
				}
				if toolCalls >= a.maxToolCalls {
					return result + "\n\n[Max tool calls reached]", nil
				}
				messages = append(messages, llm.Message{Role: "assistant", Content: resp.Content})
				messages = append(messages, llm.Message{Role: "user", Content: a.protocolContinuationPrompt(result)})
				continue
			}
		}

		calls := a.extractToolCalls(resp)
		if len(calls) == 0 {
			return resp.Content, nil
		}

		if toolCalls >= a.maxToolCalls {
			return resp.Content + "\n\n[Max tool calls reached]", nil
		}

		toolMessages := make([]llm.Message, 0, len(calls)+1)
		results := make([]string, 0, len(calls))
		assistantCallMsg := llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: make([]llm.ToolCall, 0, len(calls))}
		approvalHook := toolApprovalHookFromContext(ctx)
		for _, tc := range calls {
			currentToolCall := llm.ToolCall{ID: tc.ID, Type: "function", Function: llm.FunctionCall{Name: tc.Name, Arguments: mustJSON(tc.Args)}}
			if approvalHook != nil {
				if err := approvalHook(ctx, tc); err != nil {
					return "", &ApprovalPauseError{
						State: ApprovalResumeState{
							Messages:         cloneLLMMessages(messages),
							AssistantMessage: llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: append(append([]llm.ToolCall(nil), assistantCallMsg.ToolCalls...), currentToolCall)},
							ToolMessages:     cloneLLMMessages(toolMessages),
							Results:          append([]string(nil), results...),
							PendingTool:      cloneToolCall(tc),
						},
						Cause: err,
					}
				}
			}
			assistantCallMsg.ToolCalls = append(assistantCallMsg.ToolCalls, currentToolCall)
			if result, err := a.executeTool(ctx, tc); err != nil {
				results = append(results, fmt.Sprintf("[%s] Error: %v", tc.Name, err))
				a.recordToolActivity(ToolActivity{ToolName: tc.Name, Args: tc.Args, Error: err.Error()})
				toolMessages = append(toolMessages, llm.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: fmt.Sprintf("error: %v", err)})
			} else {
				results = append(results, fmt.Sprintf("[%s] %s", tc.Name, result))
				a.recordToolActivity(ToolActivity{ToolName: tc.Name, Args: tc.Args, Result: result})
				toolMessages = append(toolMessages, llm.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: result})
			}
		}

		messages = append(messages, assistantCallMsg)
		messages = append(messages, toolMessages...)
		messages = append(messages, llm.Message{Role: "user", Content: a.toolContinuationPrompt(results)})
	}
}

type ToolCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

func (a *Agent) parseToolCalls(content string) []ToolCall {
	var calls []ToolCall

	for _, match := range codeBlockRegex.FindAllStringSubmatch(content, -1) {
		jsonStr := strings.TrimSpace(match[1])
		if strings.HasPrefix(jsonStr, "{") {
			var tc struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
				Tool      string         `json:"tool"`
				Args      map[string]any `json:"args"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil {
				name, args := tc.Name, tc.Arguments
				if name == "" {
					name, args = tc.Tool, tc.Args
				}
				if name != "" {
					calls = append(calls, ToolCall{Name: name, Args: args})
				}
			}
		}
	}

	if len(calls) > 0 {
		return calls
	}

	for _, match := range writeFileRegex.FindAllStringSubmatch(content, -1) {
		calls = append(calls, ToolCall{
			Name: "write_file",
			Args: map[string]any{"path": match[1], "content": match[2]},
		})
	}

	for _, match := range readFileRegex.FindAllStringSubmatch(content, -1) {
		calls = append(calls, ToolCall{
			Name: "read_file",
			Args: map[string]any{"path": match[1]},
		})
	}

	for _, match := range listDirRegex.FindAllStringSubmatch(content, -1) {
		calls = append(calls, ToolCall{
			Name: "list_directory",
			Args: map[string]any{"path": match[1]},
		})
	}

	for _, match := range searchRegex.FindAllStringSubmatch(content, -1) {
		calls = append(calls, ToolCall{
			Name: "search_files",
			Args: map[string]any{"path": match[1], "pattern": match[2]},
		})
	}

	for _, match := range runCmdRegex.FindAllStringSubmatch(content, -1) {
		calls = append(calls, ToolCall{
			Name: "run_command",
			Args: map[string]any{"command": match[1]},
		})
	}

	return calls
}

func (a *Agent) extractToolCalls(resp *llm.Response) []ToolCall {
	if resp == nil {
		return nil
	}
	if len(resp.ToolCalls) > 0 {
		calls := make([]ToolCall, 0, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			args := map[string]any{}
			if strings.TrimSpace(tc.Function.Arguments) != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			id := strings.TrimSpace(tc.ID)
			if id == "" {
				id = fmt.Sprintf("toolcall_%d_%d", time.Now().UnixNano(), i)
			}
			calls = append(calls, ToolCall{ID: id, Name: tc.Function.Name, Args: args})
		}
		return calls
	}
	return a.parseToolCalls(resp.Content)
}

func (a *Agent) executeTool(ctx context.Context, tc ToolCall) (string, error) {
	if _, ok := a.tools.Get(tc.Name); !ok {
		return "", fmt.Errorf("tool not found: %s", tc.Name)
	}

	if strings.HasPrefix(tc.Name, "browser_") {
		if _, ok := tc.Args["session_id"]; !ok || strings.TrimSpace(fmt.Sprintf("%v", tc.Args["session_id"])) == "" {
			tc.Args["session_id"] = a.defaultBrowserSessionID()
		}
		ctx = tools.WithBrowserSession(ctx, fmt.Sprintf("%v", tc.Args["session_id"]))
	}

	result, err := a.tools.Call(a.toolCallContext(ctx), tc.Name, tc.Args)
	if err != nil {
		return "", fmt.Errorf("tool execution failed: %w", err)
	}

	return result, nil
}

func (a *Agent) executeProtocolResponse(ctx context.Context, resp *llm.Response) (string, bool, error) {
	if resp == nil || a.tools == nil {
		return "", false, nil
	}
	for _, payload := range extractProtocolPayloads(resp.Content) {
		result, handled, err := plugin.ExecuteProtocolOutput(ctx, a.tools, plugin.ProtocolExecutionMeta{
			ToolName: "agent_desktop_plan",
			App:      strings.TrimSpace(a.config.Name),
			Action:   "user_request",
			Input: map[string]any{
				"request": a.latestUserInput(),
			},
		}, payload)
		if handled {
			return result, true, err
		}
	}
	return "", false, nil
}

func (a *Agent) defaultBrowserSessionID() string {
	for i := len(a.history) - 1; i >= 0; i-- {
		msg := a.history[i]
		if strings.TrimSpace(msg.Role) == "user" && strings.TrimSpace(msg.Content) != "" {
			return fmt.Sprintf("agent-%s", sanitizeBrowserSessionID(msg.Content))
		}
	}
	return "agent-default"
}

func (a *Agent) latestUserInput() string {
	for i := len(a.history) - 1; i >= 0; i-- {
		if strings.TrimSpace(a.history[i].Role) == "user" && strings.TrimSpace(a.history[i].Content) != "" {
			return strings.TrimSpace(a.history[i].Content)
		}
	}
	return ""
}

func (a *Agent) protocolContinuationPrompt(result string) string {
	lines := []string{
		"Desktop plan execution result:",
		strings.TrimSpace(result),
		"",
		"Treat this as observable evidence about the current world state.",
		"Decide whether the user's requested outcome is now complete.",
		"If more work is still genuinely required, continue with the next best action or emit another desktop plan only for the remaining work.",
		"Before claiming completion, verify the requested outcome with the most reliable available checks.",
		"When you finish, provide a concise user-facing update that states what was done, what was verified, and anything still blocked or unverified.",
	}
	return strings.Join(lines, "\n")
}

func (a *Agent) toolContinuationPrompt(results []string) string {
	lines := []string{
		"Tool results above are evidence about the current world state, not proof that the task is fully complete.",
	}
	if len(results) > 0 {
		lines = append(lines, "Latest evidence:")
		for _, item := range limitStrings(results, 8) {
			lines = append(lines, "- "+item)
		}
	}
	lines = append(lines,
		"Use the observed state to decide the next step.",
		"If the requested outcome is not there yet, keep working or switch strategy instead of guessing.",
		"Before claiming completion, verify the outcome with the strongest available checks such as files, command output, browser state, UI inspection, OCR, screenshots, or app/window state.",
		"If part of the task is done but not yet verified, continue or say exactly what remains unconfirmed.",
	)
	return strings.Join(lines, "\n")
}

func (a *Agent) buildSystemPrompt() (string, error) {
	var toolList []tools.ToolInfo
	if a.tools != nil {
		toolList = a.visibleToolInfos()
	}
	return a.buildSystemPromptForToolInfos(toolList)
}

func (a *Agent) buildSystemPromptForToolInfos(toolList []tools.ToolInfo) (string, error) {
	memoryContent := a.buildPromptMemory()

	workspaceFiles := []prompt.WorkspaceFile{}
	if strings.TrimSpace(a.workingDir) != "" {
		files, err := a.loadBootstrapFiles()
		if err == nil {
			workspaceFiles = make([]prompt.WorkspaceFile, 0, len(files))
			for _, file := range files {
				workspaceFiles = append(workspaceFiles, prompt.WorkspaceFile{
					Name:    file.Name,
					Content: file.Content,
				})
			}
			if workspace.HasInjectedMemoryFile(files) && strings.Contains(strings.TrimSpace(memoryContent), "(No entries)") {
				memoryContent = ""
			}
		}
	}

	toolInfos := make([]prompt.ToolInfo, len(toolList))
	for i, t := range toolList {
		toolInfos[i] = prompt.ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	var skillPrompts []string
	if a.skills != nil {
		skillPrompts = a.skills.GetSystemPrompts()
	}

	var cliHubInfo *prompt.CLIHubInfo
	if hubRoot := strings.TrimSpace(firstNonEmpty(a.workingDir, a.workDir)); hubRoot != "" {
		if catalog, err := clihub.LoadAuto(hubRoot); err == nil {
			summary := clihub.SummaryFor(catalog, 6)
			skills := clihub.LoadSkillsForCatalog(catalog)
			var skillCommands []string
			for name, skill := range skills {
				for _, cmd := range skill.Commands {
					skillCommands = append(skillCommands, fmt.Sprintf("%s_%s", name, cmd.Name))
				}
			}
			cliHubInfo = &prompt.CLIHubInfo{
				Root:           summary.Root,
				EntriesCount:   summary.EntriesCount,
				RunnableCount:  summary.RunnableCount,
				InstalledCount: summary.InstalledCount,
				Categories:     categoryNames(summary.Categories, 6),
				Runnable:       cliHubEntryNames(summary.Runnable, 8),
				Installed:      cliHubEntryNames(summary.Installed, 6),
				SkillCommands:  skillCommands,
			}
			if reg, err := clihub.LoadCapabilityRegistry(summary.Root); err == nil {
				cliHubInfo.CapabilitiesCount = reg.Count()
				cliHubInfo.IntentExamples = cliHubCapabilityExamples(reg.All(), 6)
			}
		}
	}

	var bridgeInfo *prompt.ClawBridgeInfo
	if bridgeRoot := strings.TrimSpace(firstNonEmpty(a.workingDir, a.workDir)); bridgeRoot != "" {
		if bridgeSummary, err := clawbridge.LoadAuto(bridgeRoot); err == nil {
			bridgeInfo = &prompt.ClawBridgeInfo{
				Root:            bridgeSummary.Root,
				CommandsCount:   bridgeSummary.CommandsCount,
				ToolsCount:      bridgeSummary.ToolsCount,
				SubsystemsCount: len(bridgeSummary.Subsystems),
				CommandFamilies: familyNames(bridgeSummary.CommandFamily, 6),
				ToolFamilies:    familyNames(bridgeSummary.ToolFamily, 6),
				Subsystems:      subsystemNames(bridgeSummary.Subsystems, 5),
			}
		}
	}

	description := strings.TrimSpace(strings.Join(compactStrings(a.config.Description, a.config.Personality), "\n\n"))
	data := prompt.PromptData{
		Name:           a.config.Name,
		Description:    description,
		WorkingDir:     a.workingDir,
		Memory:         memoryContent,
		SkillPrompts:   skillPrompts,
		Tools:          toolInfos,
		CLIHub:         cliHubInfo,
		ClawBridge:     bridgeInfo,
		WorkspaceFiles: workspaceFiles,
		History:        a.history,
	}

	return prompt.BuildSystemPrompt(a.config.Name, description, data)
}

func (a *Agent) buildPromptMemory() string {
	if a.memory == nil {
		return ""
	}

	entries, err := a.memory.List()
	if err != nil || len(entries) == 0 {
		return ""
	}

	selected := selectPromptMemoryEntries(entries)
	if len(selected) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Memory\n\n")

	added := 0
	for _, entry := range selected {
		if added >= promptMemoryMaxEntries {
			break
		}
		content := strings.TrimSpace(entry.Content)
		if content == "" {
			continue
		}
		content = truncatePromptMemoryString(content, promptMemoryEntryMaxChars)
		block := fmt.Sprintf("## [%s] %s\n\n%s\n\n", entry.Type, entry.Timestamp.Format("2006-01-02 15:04"), content)
		if sb.Len()+len(block) > promptMemoryMaxChars {
			break
		}
		sb.WriteString(block)
		added++
	}

	if added == 0 {
		return ""
	}

	if len(entries) > added {
		notice := "_Prompt memory limited. Use memory_search or memory_get when older context is needed._"
		if sb.Len()+len(notice)+2 <= promptMemoryMaxChars {
			sb.WriteString(notice)
		}
	}

	return strings.TrimSpace(sb.String())
}

func selectPromptMemoryEntries(entries []memory.MemoryEntry) []memory.MemoryEntry {
	if len(entries) == 0 {
		return nil
	}

	selected := make([]memory.MemoryEntry, 0, promptMemoryMaxEntries)
	for _, entry := range entries {
		if entry.Type == memory.TypeConversation {
			continue
		}
		selected = append(selected, entry)
		if len(selected) >= promptMemoryMaxEntries {
			break
		}
	}
	return selected
}

func truncatePromptMemoryString(input string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(input))
	if len(runes) <= limit {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func (a *Agent) selectToolInfos(userInput string) []tools.ToolInfo {
	if a.tools == nil {
		return nil
	}

	allTools := a.visibleToolInfos()
	if len(allTools) == 0 {
		return nil
	}

	query := normalizeToolSelectionText(userInput)
	if !shouldExposeToolsForInput(query, allTools) && !a.shouldExposeCLIHubIntentTools(userInput) {
		return nil
	}

	coreExact := map[string]struct{}{
		"read_file":                {},
		"write_file":               {},
		"list_directory":           {},
		"search_files":             {},
		"run_command":              {},
		"memory_search":            {},
		"memory_get":               {},
		"web_search":               {},
		"fetch_url":                {},
		"clihub_catalog":           {},
		"clihub_exec":              {},
		"intent_route":             {},
		"intent_list_capabilities": {},
		"claw_bridge_context":      {},
		"delegate_task":            {},
	}
	corePrefixes := []string{"browser_", "desktop_", "skill_"}
	appPrefixes := matchedToolPrefixes(query, allTools)

	selected := make([]tools.ToolInfo, 0, len(allTools))
	seen := make(map[string]struct{})
	for _, tool := range allTools {
		if _, ok := coreExact[tool.Name]; ok {
			selected = append(selected, tool)
			seen[tool.Name] = struct{}{}
			continue
		}
		if hasAnyToolPrefix(tool.Name, corePrefixes) || hasAnyToolPrefix(tool.Name, appPrefixes) {
			if _, ok := seen[tool.Name]; ok {
				continue
			}
			selected = append(selected, tool)
			seen[tool.Name] = struct{}{}
		}
	}

	return selected
}

func (a *Agent) shouldExposeCLIHubIntentTools(userInput string) bool {
	if a.intentPreprocessor == nil {
		return false
	}
	result := a.intentPreprocessor.Preprocess(userInput, nil)
	return result != nil && result.Handled && result.Capability != nil && result.Confidence >= intentCapabilityThreshold
}

func (a *Agent) tryAutoRouteCLIHubIntent(ctx context.Context, userInput string) (string, bool, error) {
	if a.intentPreprocessor == nil || a.tools == nil {
		return "", false, nil
	}
	if _, ok := a.tools.Get("intent_route"); !ok {
		return "", false, nil
	}

	result := a.intentPreprocessor.Preprocess(userInput, nil)
	if result == nil || !result.Handled || result.Capability == nil || result.Confidence < intentAutoExecuteThreshold {
		return "", false, nil
	}

	args := map[string]any{
		"intent": strings.TrimSpace(userInput),
		"json":   true,
	}
	execResult, err := a.tools.Call(a.toolCallContext(ctx), "intent_route", args)
	if err != nil {
		a.recordToolActivity(ToolActivity{ToolName: "intent_route", Args: args, Error: err.Error()})
		return "", false, nil
	}

	a.recordToolActivity(ToolActivity{ToolName: "intent_route", Args: args, Result: execResult})
	a.appendHistoryMessage(ctx, "user", userInput)
	a.appendHistoryMessage(ctx, "assistant", execResult)
	a.recordConversation(userInput, execResult)
	return execResult, true, nil
}

func shouldExposeToolsForInput(query string, toolList []tools.ToolInfo) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}
	if matched := matchedToolPrefixes(query, toolList); len(matched) > 0 {
		return true
	}
	if strings.Contains(query, "http://") || strings.Contains(query, "https://") {
		return true
	}
	if strings.Contains(query, `\`) || strings.Contains(query, "/") || strings.Contains(query, ".md") || strings.Contains(query, ".go") || strings.Contains(query, ".json") {
		return true
	}
	if containsCJKActionIntent(query) {
		return true
	}
	if containsNaturalActionIntent(query) {
		return true
	}

	keywords := []string{
		"open", "read", "write", "edit", "update", "change", "create", "delete", "remove", "search", "find",
		"run", "execute", "list", "browse", "click", "type", "send", "install", "fix", "implement", "code",
		"file", "folder", "directory", "command", "terminal", "shell", "browser", "app", "website", "url",
		"ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â°ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦ÃƒÂ¢Ã¢â€šÂ¬Ã…â€œÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¼ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¯ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã¢â‚¬Å“", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¾ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¼ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¾ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¹Ãƒâ€¦Ã¢â‚¬Å“", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¿ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â®ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¹", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂºÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â´ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â°", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¹ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂºÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Âº", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¹ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â©ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¾ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â´ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¸ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â°ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¾", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¿ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â‚¬Å¾Ã‚Â¢", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â°ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â‚¬Å¾Ã‚Â¢", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¹ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Âº",
		"ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂµÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¹ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â ", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¹ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¾ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦ÃƒÂ¢Ã¢â€šÂ¬Ã…â€œÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¹Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â©ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â®ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â°ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â£ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¿ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â®ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â®ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¾ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â½ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â°", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â£ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¶", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂºÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â®ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â½ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¶ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¹", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¹Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â½ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¤", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â»ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¹ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â«ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¯",
		"ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂµÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¹ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¾ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂºÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â½ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¹Ãƒâ€¦Ã¢â‚¬Å“ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â§ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â«ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¾ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â©ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦ÃƒÂ¢Ã¢â€šÂ¬Ã…â€œÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¾ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â½ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¦ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¹ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂªÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚ÂºÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¾", "ocr", "ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¨ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â®ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â°ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¥ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¿ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â ", "memory",
	}
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(query, keyword) {
			return true
		}
	}
	return false
}

func containsCJKActionIntent(query string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}

	hasCJK := false
	for _, r := range query {
		if unicode.Is(unicode.Han, r) {
			hasCJK = true
			break
		}
	}
	if !hasCJK {
		return false
	}

	actionTerms := []string{
		"打开", "启动", "运行", "执行", "搜索", "查找", "读取", "查看",
		"编辑", "修改", "创建", "删除", "发送", "点击", "输入", "截图",
		"浏览", "访问", "安装", "修复", "实现", "编写", "帮我", "请帮我",
	}
	for _, term := range actionTerms {
		if strings.Contains(query, term) {
			return true
		}
	}

	// Keep a clean UTF-8 fallback list for common Chinese action verbs.
	// Some older entries above came from legacy prompt heuristics and do not
	// reliably cover everyday phrasings such as "建立一个文件夹".
	fallbackActionTerms := []string{
		"打开", "启动", "运行", "执行", "搜索", "查找", "读取", "查看",
		"编辑", "修改", "创建", "建立", "新建", "删除", "发送", "点击",
		"输入", "截图", "浏览", "访问", "安装", "修复", "实现", "编写",
		"帮我", "请帮我",
	}
	for _, term := range fallbackActionTerms {
		if strings.Contains(query, term) {
			return true
		}
	}

	return false
}

func containsNaturalActionIntent(query string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}

	keywords := []string{
		"export", "render", "timeline", "project", "model", "spreadsheet", "presentation",
		"导出", "渲染", "时间线", "项目", "模型", "表格", "演示文稿", "视频", "音频",
	}
	for _, keyword := range keywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}
	return false
}

func matchedToolPrefixes(query string, toolList []tools.ToolInfo) []string {
	if strings.TrimSpace(query) == "" {
		return nil
	}

	prefixes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, tool := range toolList {
		prefix := toolPrefix(tool.Name)
		if prefix == "" || isCoreToolPrefix(prefix) {
			continue
		}
		normalizedPrefix := normalizeToolSelectionText(prefix)
		if normalizedPrefix != "" && strings.Contains(query, normalizedPrefix) {
			if _, ok := seen[prefix]; ok {
				continue
			}
			seen[prefix] = struct{}{}
			prefixes = append(prefixes, prefix+"_")
		}
	}
	return prefixes
}

func toolPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	idx := strings.Index(name, "_")
	if idx <= 0 {
		return ""
	}
	return strings.TrimSpace(name[:idx])
}

func isCoreToolPrefix(prefix string) bool {
	switch prefix {
	case "browser", "desktop", "skill", "memory", "intent", "clihub", "claw", "read", "write", "list", "search", "run", "web", "fetch":
		return true
	default:
		return false
	}
}

func hasAnyToolPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func normalizeToolSelectionText(input string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ", `\`, " ", ".", " ", ":", " ")
	return strings.ToLower(strings.TrimSpace(replacer.Replace(input)))
}

func (a *Agent) visibleToolInfos() []tools.ToolInfo {
	if a.tools == nil {
		return nil
	}
	return a.tools.ListForRole(a.config.IsSubAgent)
}

func (a *Agent) toolCallContext(ctx context.Context) context.Context {
	role := tools.ToolCallerRoleMainAgent
	if a.config.IsSubAgent {
		role = tools.ToolCallerRoleSubAgent
	}
	return tools.WithToolCaller(ctx, tools.ToolCaller{
		Role:      role,
		AgentName: strings.TrimSpace(a.config.Name),
	})
}

func (a *Agent) buildMessages(systemPrompt string) []llm.Message {
	messages := make([]llm.Message, 0, 2+len(a.history))
	if systemPrompt != "" {
		messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	}
	for _, msg := range a.history {
		messages = append(messages, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	return messages
}

func (a *Agent) buildToolDefinitions() []llm.ToolDefinition {
	if a.tools == nil {
		return nil
	}
	return buildToolDefinitionsFromInfos(a.visibleToolInfos())
}

func buildToolDefinitionsFromInfos(toolList []tools.ToolInfo) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(toolList))
	for _, t := range toolList {
		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return defs
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func familyNames(items []clawbridge.FamilySummary, limit int) []string {
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s (%d)", item.Name, item.Count))
	}
	return names
}

func subsystemNames(items []clawbridge.SubsystemSummary, limit int) []string {
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s (%d)", item.Name, item.ModuleCount))
	}
	return names
}

func categoryNames(items []clihub.CategoryHit, limit int) []string {
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s (%d)", item.Name, item.Count))
	}
	return names
}

func cliHubEntryNames(items []clihub.EntryStatus, limit int) []string {
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

func cliHubCapabilityExamples(items []clihub.Capability, limit int) []string {
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	examples := make([]string, 0, len(items))
	for _, item := range items {
		parts := compactStrings(item.Harness, item.Group, item.Command)
		if len(parts) == 0 {
			continue
		}
		examples = append(examples, strings.Join(parts, " "))
	}
	return examples
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sanitizeBrowserSessionID(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return "default"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", "?", "", "&", "-")
	input = replacer.Replace(input)
	if len(input) > 48 {
		input = input[:48]
	}
	input = strings.Trim(input, "-.")
	if input == "" {
		return "default"
	}
	return input
}

func extractProtocolPayloads(content string) [][]byte {
	items := make([][]byte, 0, 2)
	seen := map[string]bool{}
	appendCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		items = append(items, []byte(candidate))
	}
	appendCandidate(content)
	for _, match := range codeBlockRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			appendCandidate(match[1])
		}
	}
	return items
}

func compactStrings(items ...string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func limitStrings(items []string, limit int) []string {
	if limit <= 0 || len(items) <= limit {
		return append([]string(nil), items...)
	}
	result := append([]string(nil), items[:limit]...)
	result = append(result, fmt.Sprintf("...and %d more result(s)", len(items)-limit))
	return result
}

func (a *Agent) ShowMemory() (string, error) {
	return memory.NewMemoryService(a.memory).FormatAsMarkdown()
}

func (a *Agent) recordConversation(userInput string, response string) {
	if a.memory == nil {
		return
	}
	_ = a.memory.Add(memory.MemoryEntry{Type: "conversation", Role: "user", Content: userInput})
	_ = a.memory.Add(memory.MemoryEntry{Type: "conversation", Role: "assistant", Content: response})
}

func (a *Agent) ListSkills() []skills.SkillInfo {
	list := a.skills.List()
	result := make([]skills.SkillInfo, len(list))
	for i, s := range list {
		result[i] = skills.SkillInfo{Name: s.Name, Description: s.Description, Version: s.Version, Permissions: append([]string(nil), s.Permissions...), Entrypoint: s.Entrypoint, Registry: s.Registry, Source: s.Source, InstallHint: s.InstallCommand}
	}
	return result
}

func (a *Agent) ListTools() []tools.ToolInfo {
	if a.tools == nil {
		return nil
	}
	return a.visibleToolInfos()
}

func (a *Agent) ClearHistory() {
	a.history = a.history[:0]
}

func (a *Agent) GetHistory() []prompt.Message {
	return a.history
}

func (a *Agent) SetHistory(history []prompt.Message) {
	if len(history) == 0 {
		a.history = nil
		return
	}
	a.history = append([]prompt.Message(nil), history...)
}

func (a *Agent) SetTools(registry *tools.Registry) {
	a.tools = registry
}

func (a *Agent) SetContextEngine(engine ctxpkg.ContextEngine) {
	if a.contextRuntime == nil {
		a.contextRuntime = newContextRuntime(a.config)
	}
	a.contextRuntime.setContextEngine(engine)
}

func (a *Agent) appendHistoryMessage(ctx context.Context, role string, content string) {
	a.history = append(a.history, prompt.Message{Role: role, Content: content})
	if a.contextRuntime != nil {
		a.contextRuntime.storeConversation(ctx, a.config.Name, a.workingDir, role, content)
	}
}

func (a *Agent) loadBootstrapFiles() ([]workspace.BootstrapFile, error) {
	if a.contextRuntime != nil {
		return a.contextRuntime.loadBootstrapFiles(a.workingDir)
	}
	return workspace.LoadBootstrapFiles(a.workingDir, workspace.BootstrapOptions{})
}

func (a *Agent) prepareSystemPrompt(ctx context.Context, selectedTools []tools.ToolInfo) (string, error) {
	if a.contextRuntime != nil {
		compacted, err := a.contextRuntime.compactHistory(ctx, a.history, a.llm)
		if err != nil {
			return "", fmt.Errorf("failed to compact history: %w", err)
		}
		a.history = compacted
	}

	systemPrompt, err := a.buildSystemPromptForToolInfos(selectedTools)
	if err != nil {
		return "", fmt.Errorf("failed to build system prompt: %w", err)
	}

	if a.contextRuntime != nil {
		trimmed, err := a.contextRuntime.enforceTokenLimit(systemPrompt, a.history)
		if err != nil {
			return "", err
		}
		a.history = trimmed
	}

	return systemPrompt, nil
}

func (a *Agent) acquireContextExecution(ctx context.Context, userInput string) (*contextExecution, error) {
	if a.contextRuntime == nil {
		return &contextExecution{}, nil
	}
	return a.contextRuntime.acquire(ctx, firstNonEmpty(a.config.Name, userInput))
}

func (a *Agent) heartbeatContextExecution(exec *contextExecution) {
	if a.contextRuntime == nil {
		return
	}
	a.contextRuntime.heartbeat(exec)
}

func (a *Agent) releaseContextExecution(exec *contextExecution) {
	if a.contextRuntime == nil {
		return
	}
	a.contextRuntime.release(exec)
}
