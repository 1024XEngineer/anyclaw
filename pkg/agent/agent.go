package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/prompt"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type LLMCaller interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error)
	Name() string
}

type Agent struct {
	config             Config
	llm                LLMCaller
	memory             *memory.FileMemory
	skills             *skills.SkillsManager
	tools              *tools.Registry
	workDir            string
	history            []prompt.Message
	maxToolCalls       int
	observer           Observer
	observerMu         sync.RWMutex
	lastToolActivities []ToolActivity
}

type Config struct {
	Name        string
	Description string
	Personality string
	LLM         LLMCaller
	Memory      *memory.FileMemory
	Skills      *skills.SkillsManager
	Tools       *tools.Registry
	WorkDir     string
}

var (
	codeBlockRegex = regexp.MustCompile("(?s)```(?:json)?[\\s]*(.+?)[\\s]*```")
	writeFileRegex = regexp.MustCompile("write_file\\s+path\\s*=\\s*\"([^\"]+)\"\\s+content\\s*=\\s*\"([\\s\\S]*?)\"")
	readFileRegex  = regexp.MustCompile("read_file\\s+path\\s*=\\s*\"([^\"]+)\"")
	listDirRegex   = regexp.MustCompile("list_directory\\s+path\\s*=\\s*\"([^\"]+)\"")
	searchRegex    = regexp.MustCompile("search_files\\s+path\\s*=\\s*\"([^\"]+)\"\\s+pattern\\s*=\\s*\"([^\"]+)\"")
	runCmdRegex    = regexp.MustCompile("run_command\\s+command\\s*=\\s*\"([^\"]+)\"")
)

func New(cfg Config) *Agent {
	return &Agent{
		config:       cfg,
		llm:          cfg.LLM,
		memory:       cfg.Memory,
		skills:       cfg.Skills,
		tools:        cfg.Tools,
		workDir:      cfg.WorkDir,
		history:      []prompt.Message{},
		maxToolCalls: 10,
	}
}

func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	a.resetToolActivities()
	systemPrompt, err := a.buildSystemPrompt()
	if err != nil {
		return "", fmt.Errorf("failed to build system prompt: %w", err)
	}

	a.history = append(a.history, prompt.Message{Role: "user", Content: userInput})
	messages := a.buildMessages(systemPrompt)
	toolDefs := a.buildToolDefinitions()

	response, err := a.chatWithTools(ctx, messages, toolDefs)
	if err != nil {
		return "", err
	}

	a.history = append(a.history, prompt.Message{Role: "assistant", Content: response})

	a.memory.Add(memory.MemoryEntry{Type: "conversation", Role: "user", Content: userInput})
	a.memory.Add(memory.MemoryEntry{Type: "conversation", Role: "assistant", Content: response})

	return response, nil
}

func (a *Agent) chatWithTools(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (string, error) {
	for toolCalls := 0; ; toolCalls++ {
		resp, err := a.llm.Chat(ctx, messages, toolDefs)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		calls := a.extractToolCalls(resp)
		if len(calls) == 0 {
			return resp.Content, nil
		}

		if toolCalls >= a.maxToolCalls {
			return resp.Content + "\n\n[Max tool calls reached]", nil
		}

		toolMessages := make([]llm.Message, 0, len(calls)+1)
		results := make([]string, len(calls))
		assistantCallMsg := llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: make([]llm.ToolCall, 0, len(calls))}
		approvalHook := toolApprovalHookFromContext(ctx)
		for i, tc := range calls {
			if approvalHook != nil {
				if err := approvalHook(ctx, tc); err != nil {
					return "", err
				}
			}
			assistantCallMsg.ToolCalls = append(assistantCallMsg.ToolCalls, llm.ToolCall{ID: tc.ID, Type: "function", Function: llm.FunctionCall{Name: tc.Name, Arguments: mustJSON(tc.Args)}})
			if result, err := a.executeTool(ctx, tc); err != nil {
				results[i] = fmt.Sprintf("[%s] Error: %v", tc.Name, err)
				a.recordToolActivity(ToolActivity{ToolName: tc.Name, Args: tc.Args, Error: err.Error()})
				toolMessages = append(toolMessages, llm.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: fmt.Sprintf("error: %v", err)})
			} else {
				results[i] = fmt.Sprintf("[%s] %s", tc.Name, result)
				a.recordToolActivity(ToolActivity{ToolName: tc.Name, Args: tc.Args, Result: result})
				toolMessages = append(toolMessages, llm.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: result})
			}
		}

		messages = append(messages, assistantCallMsg)
		messages = append(messages, toolMessages...)
		if len(toolMessages) == 0 {
			messages = append(messages, llm.Message{Role: "user", Content: fmt.Sprintf("Tool results:\n%s\n\nContinue if needed.", strings.Join(results, "\n"))})
		}
	}
}

type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
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

	result, err := a.tools.Call(ctx, tc.Name, tc.Args)
	if err != nil {
		return "", fmt.Errorf("tool execution failed: %w", err)
	}

	return result, nil
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

func (a *Agent) buildSystemPrompt() (string, error) {
	memoryContent, _ := a.memory.FormatAsMarkdown()

	toolList := a.tools.List()
	toolInfos := make([]prompt.ToolInfo, len(toolList))
	for i, t := range toolList {
		toolInfos[i] = prompt.ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	data := prompt.PromptData{
		Name:         a.config.Name,
		Description:  strings.TrimSpace(strings.Join([]string{a.config.Description, a.config.Personality}, "\n\n")),
		Memory:       memoryContent,
		SkillPrompts: a.skills.GetSystemPrompts(),
		Tools:        toolInfos,
		History:      a.history,
	}

	return prompt.BuildSystemPrompt(a.config.Name, a.config.Description, data)
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
	toolList := a.tools.List()
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

func (a *Agent) ShowMemory() (string, error) {
	return a.memory.FormatAsMarkdown()
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
	return a.tools.List()
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
