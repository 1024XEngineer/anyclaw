package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	ctxpkg "github.com/1024XEngineer/anyclaw/pkg/runtime/context/store"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/workspace"
)

type LLMCaller interface {
	Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error)
	StreamChat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) error
	Name() string
}

type LLMStreamResponder interface {
	StreamChatResponse(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(string)) (*llm.Response, error)
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
	contextRuntime     *contextRuntime
}

type Config struct {
	Name         string
	Description  string
	Personality  string
	SystemPrompt string
	IsSubAgent   bool
	LLM          LLMCaller
	Memory       memory.MemoryBackend
	Skills       *skills.SkillsManager
	Tools        *tools.Registry
	WorkDir      string
	WorkingDir   string
	CLIHubRoot   string

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
	codexMaxSelectedTools                = 18
	codexSelectedToolDefinitionBudget    = 1800
	codexMaxToolDescriptionChars         = 220
	codexMaxToolSchemaBytes              = 3600
	codexMaxCompactToolSchemaProperties  = 24
	codexMaxCompactToolPropertyDescChars = 120
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
	capabilityPlan := a.planCapability(userInput)
	selectedTools := a.selectToolInfosWithPlan(userInput, capabilityPlan)
	a.appendHistoryMessage(ctx, "user", userInput)
	toolDefs := a.buildSelectedToolDefinitions(selectedTools)
	systemPrompt, err := a.prepareSystemPrompt(ctx, selectedTools, toolDefs, capabilityPlan)
	if err != nil {
		return "", err
	}
	messages := a.buildMessages(systemPrompt)
	messages = a.appendCapabilityPlanningContext(ctx, messages, capabilityPlan, selectedTools)

	a.heartbeatContextExecution(exec)
	response, err := NewCodexChain(a).Run(ctx, CodexChainRequest{
		Messages: messages,
		Tools:    toolDefs,
	})
	if err != nil {
		return "", err
	}

	a.appendHistoryMessage(ctx, "assistant", response)

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

	capabilityPlan := a.planCapability(userInput)
	selectedTools := a.selectToolInfosWithPlan(userInput, capabilityPlan)
	a.appendHistoryMessage(ctx, "user", userInput)
	toolDefs := a.buildSelectedToolDefinitions(selectedTools)
	systemPrompt, err := a.prepareSystemPrompt(ctx, selectedTools, toolDefs, capabilityPlan)
	if err != nil {
		return err
	}
	messages := a.buildMessages(systemPrompt)
	messages = a.appendCapabilityPlanningContext(ctx, messages, capabilityPlan, selectedTools)

	a.heartbeatContextExecution(exec)
	response, err := NewCodexChain(a).Run(ctx, CodexChainRequest{
		Messages: messages,
		Tools:    toolDefs,
		OnChunk:  onChunk,
	})
	if err != nil {
		return err
	}
	a.appendHistoryMessage(ctx, "assistant", response)

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
	return result.Response, true, nil
}

type ToolCall struct {
	ID               string         `json:"id,omitempty"`
	Name             string         `json:"name"`
	Args             map[string]any `json:"args,omitempty"`
	RequiresApproval bool           `json:"requires_approval,omitempty"`
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

func (a *Agent) toolRequiresApproval(name string) bool {
	if a == nil || a.tools == nil {
		return false
	}
	return a.tools.RequiresApproval(name)
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
		"When you finish, provide a Codex-style user-facing update with 已处理, 已验证, and 未确认/阻塞 sections when responding in Chinese.",
	}
	return strings.Join(lines, "\n")
}

func (a *Agent) toolContinuationPrompt(observations []ToolObservation) string {
	lines := []string{
		"Structured tool observations above are evidence about the current world state, not proof that the task is fully complete.",
	}
	if len(observations) > 0 {
		lines = append(lines, "Latest structured evidence:")
		for _, obs := range limitToolObservations(observations, 8) {
			status := "ok"
			if !obs.OK {
				status = "error"
			}
			summary := strings.TrimSpace(obs.Summary)
			if summary == "" {
				summary = strings.TrimSpace(obs.Output)
			}
			if summary == "" && obs.Error != "" {
				summary = obs.Error
			}
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", status, obs.Tool, truncateString(summary, codexLoopMaxSummaryChars)))
		}
	}
	lines = append(lines,
		"Use the observed state to decide the next step.",
		"If the requested outcome is not there yet, keep working or switch strategy instead of guessing.",
		"Before claiming completion, verify the outcome with the strongest available checks such as files, command output, browser state, UI inspection, OCR, screenshots, or app/window state.",
		"If part of the task is done but not yet verified, continue or say exactly what remains unconfirmed.",
		"When finished, answer like Codex: summarize 已处理, 已验证, and 未确认/阻塞 instead of only saying it is done.",
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

func (a *Agent) BuildSystemPrompt() (string, error) {
	return a.buildSystemPrompt()
}

func (a *Agent) buildSystemPromptForToolInfos(toolList []tools.ToolInfo) (string, error) {
	return a.buildSystemPromptForToolInfosWithPlan(toolList, nil)
}

func (a *Agent) buildSystemPromptForToolInfosWithPlan(toolList []tools.ToolInfo, capabilityPlan *CapabilityPlan) (string, error) {
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

	var availableSkills []prompt.AvailableSkill
	if a.skills != nil {
		availableSkills = a.availableSkillsForPrompt()
	}

	var cliHubInfo *prompt.CLIHubInfo
	if shouldIncludeCLIHubPromptInfo(toolList) {
		hubRoot := strings.TrimSpace(firstNonEmpty(a.workingDir, a.workDir))
		if hubRoot != "" {
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
	}

	var bridgeInfo *prompt.ClawBridgeInfo
	if shouldIncludeClawBridgePromptInfo(toolList) {
		bridgeRoot := strings.TrimSpace(firstNonEmpty(a.workingDir, a.workDir))
		if bridgeRoot != "" {
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
	}

	description := strings.TrimSpace(strings.Join(compactStrings(a.config.Description, a.config.Personality), "\n\n"))
	data := prompt.PromptData{
		Name:            a.config.Name,
		Description:     description,
		SystemPrompt:    strings.TrimSpace(a.config.SystemPrompt),
		Personality:     strings.TrimSpace(a.config.Personality),
		WorkingDir:      a.workingDir,
		AvailableSkills: availableSkills,
		SkillPrompts:    a.selectedSkillPrompts(toolList),
		Tools:           toolInfos,
		CLIHub:          cliHubInfo,
		ClawBridge:      bridgeInfo,
		WorkspaceFiles:  workspaceFiles,
		History:         a.history,
	}
	if capabilityPlan != nil {
		data.CapabilityPlan = prompt.CapabilityPlanInfo{
			TaskClass:                capabilityPlan.TaskClass,
			Route:                    capabilityPlan.Route,
			Need:                     capabilityPlan.Need,
			KindHint:                 capabilityPlan.KindHint,
			ShouldExposeMarketSearch: capabilityPlan.ShouldExposeMarketSearch,
			Reason:                   capabilityPlan.Reason,
		}
		if len(capabilityPlan.LocalMatches) > 0 {
			match := capabilityPlan.LocalMatches[0]
			data.CapabilityPlan.TopLocalMatch = prompt.CapabilityMatchInfo{
				Kind:   match.Kind,
				Name:   match.Name,
				Score:  match.Score,
				Reason: match.Reason,
			}
		}
	}

	return prompt.BuildSystemPrompt(a.config.Name, description, data)
}

func (a *Agent) availableSkillsForPrompt() []prompt.AvailableSkill {
	if a.skills == nil {
		return nil
	}
	list := append([]*skills.Skill(nil), a.skills.List()...)
	sort.Slice(list, func(i, j int) bool {
		left := ""
		if list[i] != nil {
			left = strings.ToLower(strings.TrimSpace(list[i].Name))
		}
		right := ""
		if list[j] != nil {
			right = strings.ToLower(strings.TrimSpace(list[j].Name))
		}
		return left < right
	})

	items := make([]prompt.AvailableSkill, 0, len(list))
	for _, skill := range list {
		if skill == nil {
			continue
		}
		location := readableSkillLocation(skill)
		if location == "" {
			continue
		}
		items = append(items, prompt.AvailableSkill{
			Name:        skill.Name,
			Description: skill.Description,
			Location:    location,
		})
	}
	return items
}

func readableSkillLocation(skill *skills.Skill) string {
	if skill == nil || skill.Metadata == nil {
		return ""
	}
	skillPath := strings.TrimSpace(skill.Metadata["path"])
	if skillPath == "" || strings.HasPrefix(strings.ToLower(skillPath), "builtin://") {
		return ""
	}
	if !filepath.IsAbs(skillPath) {
		if absPath, err := filepath.Abs(skillPath); err == nil {
			skillPath = absPath
		}
	}
	if info, err := os.Stat(skillPath); err == nil && !info.IsDir() {
		return filepath.Clean(skillPath)
	}
	for _, name := range []string{"SKILL.md", "skill.json"} {
		candidate := filepath.Join(skillPath, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

func (a *Agent) selectToolInfos(userInput string) []tools.ToolInfo {
	return a.selectToolInfosWithPlan(userInput, nil)
}

func (a *Agent) selectToolInfosWithPlan(userInput string, capabilityPlan *CapabilityPlan) []tools.ToolInfo {
	if a.tools == nil {
		return nil
	}

	allTools := a.visibleToolInfos()
	if len(allTools) == 0 {
		return nil
	}

	query := normalizeToolSelectionText(userInput)
	marketFollowUp := a.marketFollowUpIntent(userInput)
	cliHubIntent := a.shouldExposeCLIHubIntentTools(userInput) || capabilityPlanUsesCLI(capabilityPlan)
	toolExposure := shouldExposeToolsForInput(query, allTools)
	skillInstructions := a.hasReadableSkillInstructions()
	plannerExposure := capabilityPlanExposesTools(capabilityPlan)
	if !toolExposure && !cliHubIntent && !skillInstructions && !plannerExposure && !marketFollowUp.ShouldExpose() {
		return nil
	}

	intent := classifyCodexToolIntent(query, userInput, cliHubIntent)
	coreExact := selectedCoreToolNamesForIntent(query, userInput, intent)
	if skillInstructions {
		coreExact["read"] = struct{}{}
		coreExact["read_file"] = struct{}{}
	}
	applyCapabilityPlanToolSelection(coreExact, capabilityPlan)
	applyMarketFollowUpToolSelection(coreExact, marketFollowUp)
	prefixes := selectedToolPrefixesForIntent(query, allTools, intent)
	prefixes = append(prefixes, selectedToolPrefixesForCapabilityPlan(capabilityPlan)...)
	appPrefixes := matchedToolPrefixes(query, allTools)
	prefixes = append(prefixes, appPrefixes...)

	selected := make([]tools.ToolInfo, 0, len(allTools))
	seen := make(map[string]struct{})
	for _, tool := range allTools {
		if _, ok := coreExact[tool.Name]; ok {
			selected = append(selected, tool)
			seen[tool.Name] = struct{}{}
			continue
		}
		if hasAnyToolPrefix(tool.Name, prefixes) {
			if _, ok := seen[tool.Name]; ok {
				continue
			}
			selected = append(selected, tool)
			seen[tool.Name] = struct{}{}
		}
	}

	return limitSelectedToolInfos(selected, codexMaxSelectedTools)
}

func (a *Agent) hasReadableSkillInstructions() bool {
	if a.skills == nil {
		return false
	}
	for _, skill := range a.skills.List() {
		if readableSkillLocation(skill) != "" {
			return true
		}
	}
	return false
}

func (a *Agent) planCapability(userInput string) *CapabilityPlan {
	allTools := []tools.ToolInfo(nil)
	if a.tools != nil {
		allTools = a.visibleToolInfos()
	}
	input := CapabilityPlannerInput{
		UserInput:       userInput,
		Tools:           allTools,
		Skills:          a.skillCatalogForCapabilityPlanner(),
		CLICapabilities: a.cliCapabilitiesForCapabilityPlanner(),
		LocalArtifacts:  a.localArtifactsForCapabilityPlanner(),
	}
	plan := CapabilityPlanner{}.Plan(input)
	return &plan
}

func (a *Agent) skillCatalogForCapabilityPlanner() []skills.SkillCatalogEntry {
	if a == nil || a.skills == nil {
		return nil
	}
	return a.skills.Catalog()
}

func (a *Agent) cliCapabilitiesForCapabilityPlanner() []clihub.Capability {
	if a == nil {
		return nil
	}
	root := strings.TrimSpace(firstNonEmpty(a.config.CLIHubRoot, a.workingDir, a.workDir))
	if root == "" {
		return nil
	}
	reg, err := clihub.LoadCapabilityRegistry(root)
	if err != nil || reg == nil {
		return nil
	}
	return reg.All()
}

func (a *Agent) localArtifactsForCapabilityPlanner() []CapabilityArtifactSummary {
	if a == nil {
		return nil
	}
	root := strings.TrimSpace(firstNonEmpty(a.workingDir, a.workDir))
	if root == "" {
		return nil
	}
	store := marketplace.NewStore(root)
	if _, err := os.Stat(store.ReceiptsDir()); err != nil {
		return nil
	}
	receipts, err := store.ListReceipts()
	if err != nil {
		return nil
	}
	bound := map[string]bool{}
	if _, err := os.Stat(store.BindingsDir()); err == nil {
		if bindings, err := store.ListBindings(); err == nil {
			for _, binding := range bindings.Items {
				if binding.State == marketplace.BindingEnabled {
					bound[strings.ToLower(strings.TrimSpace(binding.ArtifactID))] = true
				}
			}
		}
	}
	items := make([]CapabilityArtifactSummary, 0, len(receipts))
	for _, receipt := range receipts {
		id := strings.TrimSpace(receipt.ArtifactID)
		if id == "" {
			continue
		}
		capabilities := append([]string(nil), receipt.Permissions...)
		if receipt.RiskLevel != "" {
			capabilities = append(capabilities, receipt.RiskLevel)
		}
		if receipt.TrustLevel != "" {
			capabilities = append(capabilities, receipt.TrustLevel)
		}
		items = append(items, CapabilityArtifactSummary{
			ArtifactID:   id,
			Kind:         string(receipt.Kind),
			Name:         receipt.Name,
			Description:  receipt.Description,
			Capabilities: capabilities,
			Installed:    true,
			Bound:        bound[strings.ToLower(id)],
		})
	}
	return items
}

func capabilityPlanExposesTools(plan *CapabilityPlan) bool {
	if plan == nil {
		return false
	}
	switch plan.Route {
	case CapabilityRouteUseSkill, CapabilityRouteDelegateAgent, CapabilityRouteUseCLI, CapabilityRouteSearchMarket:
		return true
	default:
		return plan.ShouldExposeMarketSearch
	}
}

func capabilityPlanUsesCLI(plan *CapabilityPlan) bool {
	return plan != nil && plan.Route == CapabilityRouteUseCLI
}

func applyCapabilityPlanToolSelection(selected map[string]struct{}, plan *CapabilityPlan) {
	if selected == nil || plan == nil {
		return
	}
	add := func(names ...string) {
		for _, name := range names {
			selected[name] = struct{}{}
		}
	}
	switch plan.Route {
	case CapabilityRouteDelegateAgent:
		add("delegate_task")
	case CapabilityRouteUseCLI:
		add("clihub_catalog", "clihub_exec", "intent_route", "intent_list_capabilities")
	case CapabilityRouteSearchMarket:
		if plan.ShouldExposeMarketSearch {
			add("market_search_artifacts")
		}
	}
}

func selectedToolPrefixesForCapabilityPlan(plan *CapabilityPlan) []string {
	if plan == nil {
		return nil
	}
	switch plan.Route {
	case CapabilityRouteUseSkill:
		if len(plan.LocalMatches) > 0 && strings.TrimSpace(plan.LocalMatches[0].Name) != "" {
			return []string{"skill_" + strings.TrimSpace(plan.LocalMatches[0].Name)}
		}
		return []string{"skill_"}
	default:
		return nil
	}
}

type marketFollowUpIntent struct {
	Install    bool
	Bind       bool
	ArtifactID string
}

func (m marketFollowUpIntent) ShouldExpose() bool {
	return m.Install || m.Bind
}

func (a *Agent) marketFollowUpIntent(userInput string) marketFollowUpIntent {
	combined := strings.TrimSpace(userInput + "\n" + a.recentConversationText(6))
	artifactID := firstArtifactID(combined)
	if artifactID == "" {
		return marketFollowUpIntent{}
	}
	query := normalizeCapabilityPlannerText(userInput)
	raw := strings.ToLower(strings.TrimSpace(userInput))
	install := capabilityPlannerContainsAny(query,
		"install", "install it", "install this", "install that", "go ahead", "confirmed", "i confirm", "yes install",
		"安装", "确认安装", "同意安装", "可以安装", "继续安装",
	)
	bind := capabilityPlannerContainsAny(query,
		"bind", "activate", "enable", "use it", "attach", "make it available",
		"绑定", "启用", "激活", "使用它", "挂载",
	)
	if !install && !bind {
		return marketFollowUpIntent{}
	}
	if strings.Contains(raw, "don't") || strings.Contains(raw, "do not") || strings.Contains(raw, "取消") || strings.Contains(raw, "不要") {
		return marketFollowUpIntent{}
	}
	return marketFollowUpIntent{Install: install, Bind: bind, ArtifactID: artifactID}
}

func (a *Agent) recentConversationText(limit int) string {
	if a == nil || len(a.history) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(a.history) {
		limit = len(a.history)
	}
	start := len(a.history) - limit
	parts := make([]string, 0, limit)
	for _, msg := range a.history[start:] {
		if strings.TrimSpace(msg.Content) != "" {
			parts = append(parts, msg.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func firstArtifactID(text string) string {
	for _, match := range artifactIDRegex.FindAllString(text, -1) {
		match = strings.Trim(match, ".,;:()[]{}<>\"'`")
		if isLikelyMarketplaceArtifactID(match) {
			return match
		}
	}
	return ""
}

var artifactIDRegex = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9_-]*(?:\.[a-z0-9][a-z0-9_-]*){1,}\b`)

func isLikelyMarketplaceArtifactID(value string) bool {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), ".")
	if len(parts) < 2 {
		return false
	}
	hasKind := false
	hasAlpha := false
	for _, part := range parts {
		if part == "" {
			return false
		}
		switch part {
		case "agent", "skill", "cli", "plugin":
			hasKind = true
		}
		for _, r := range part {
			if r >= 'a' && r <= 'z' {
				hasAlpha = true
				break
			}
		}
	}
	return hasKind && hasAlpha
}

func applyMarketFollowUpToolSelection(selected map[string]struct{}, intent marketFollowUpIntent) {
	if selected == nil || !intent.ShouldExpose() {
		return
	}
	if intent.Install {
		selected["market_install_artifact"] = struct{}{}
	}
	if intent.Bind {
		selected["market_bind_artifact"] = struct{}{}
	}
}

func (a *Agent) appendCapabilityPlanningContext(ctx context.Context, messages []llm.Message, plan *CapabilityPlan, selectedTools []tools.ToolInfo) []llm.Message {
	result, ok := a.searchMarketplaceForCapabilityPlan(ctx, plan, selectedTools)
	if !ok {
		return messages
	}
	return append(messages, llm.Message{Role: "user", Content: result})
}

func (a *Agent) searchMarketplaceForCapabilityPlan(ctx context.Context, plan *CapabilityPlan, selectedTools []tools.ToolInfo) (string, bool) {
	if a == nil || a.tools == nil || plan == nil || !plan.ShouldExposeMarketSearch || plan.Route != CapabilityRouteSearchMarket {
		return "", false
	}
	if !toolInfoListContains(selectedTools, "market_search_artifacts") {
		return "", false
	}
	query := strings.TrimSpace(firstNonEmpty(plan.Need, plan.Summary()))
	if query == "" {
		return "", false
	}
	args := map[string]any{
		"query": query,
		"limit": 3,
	}
	if kind := strings.TrimSpace(plan.KindHint); kind != "" {
		args["kind"] = kind
	}
	result, err := a.tools.Call(a.toolCallContext(ctx), "market_search_artifacts", args)
	if err != nil {
		a.recordToolActivity(ToolActivity{ToolName: "market_search_artifacts", Args: args, Error: err.Error()})
		return "## Capability Planning Search\nMarketplace search was attempted for the missing capability, but it failed: " + truncateString(err.Error(), 240) + "\nContinue with local tools if possible, or explain the limitation clearly.", true
	}
	a.recordToolActivity(ToolActivity{ToolName: "market_search_artifacts", Args: args, Result: result})
	return formatCapabilityPlanningSearchContext(result), true
}

func formatCapabilityPlanningSearchContext(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "## Capability Planning Search\nMarketplace search returned no usable result. Continue with local tools if possible, or explain the limitation clearly."
	}
	var payload struct {
		Route struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		} `json:"route"`
		LocalCount int                                 `json:"local_count"`
		CloudCount int                                 `json:"cloud_count"`
		CloudError string                              `json:"cloud_error"`
		Local      []capabilityPlanningSearchCandidate `json:"local"`
		Cloud      []capabilityPlanningSearchCandidate `json:"cloud"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "## Capability Planning Search\nMarketplace search result:\n" + truncateString(raw, 1600)
	}
	lines := []string{
		"## Capability Planning Search",
		fmt.Sprintf("- Search counts: %d installed/local, %d cloud.", payload.LocalCount, payload.CloudCount),
	}
	if strings.TrimSpace(payload.Route.Action) != "" {
		line := "- Recommended route: " + payload.Route.Action
		if strings.TrimSpace(payload.Route.Reason) != "" {
			line += " (" + truncateString(payload.Route.Reason, 180) + ")"
		}
		lines = append(lines, line+".")
	}
	if len(payload.Local) > 0 {
		lines = append(lines, "- Local candidates: "+formatCapabilityPlanningCandidates(payload.Local)+".")
	}
	if len(payload.Cloud) > 0 {
		lines = append(lines, "- Cloud candidates: "+formatCapabilityPlanningCandidates(payload.Cloud)+".")
	}
	if strings.TrimSpace(payload.CloudError) != "" {
		lines = append(lines, "- Cloud search error: "+truncateString(payload.CloudError, 220)+".")
	}
	lines = append(lines,
		"- Use installed/local candidates directly when they are already bound and available through current tools.",
		"- For cloud candidates, explain the option and request explicit user confirmation before installing or binding anything.",
		"- If no candidate fits, continue locally when reasonable or state the capability gap honestly.",
	)
	return strings.Join(lines, "\n")
}

type capabilityPlanningSearchCandidate struct {
	ArtifactID string `json:"artifact_id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	RiskLevel  string `json:"risk_level"`
	TrustLevel string `json:"trust_level"`
}

func formatCapabilityPlanningCandidates(items []capabilityPlanningSearchCandidate) string {
	limit := 3
	if len(items) < limit {
		limit = len(items)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		item := items[i]
		label := firstNonEmpty(item.Name, item.ArtifactID)
		if item.Kind != "" {
			label += " [" + item.Kind + "]"
		}
		meta := compactStrings(item.ArtifactID, item.RiskLevel, item.TrustLevel)
		if len(meta) > 0 {
			label += " (" + strings.Join(meta, ", ") + ")"
		}
		parts = append(parts, truncateString(label, 180))
	}
	if len(items) > limit {
		parts = append(parts, "and "+strconv.Itoa(len(items)-limit)+" more")
	}
	return strings.Join(parts, "; ")
}

func toolInfoListContains(list []tools.ToolInfo, name string) bool {
	for _, tool := range list {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func selectedCoreToolNames(query string, rawInput string, cliHubIntent bool) map[string]struct{} {
	return selectedCoreToolNamesForIntent(query, rawInput, classifyCodexToolIntent(query, rawInput, cliHubIntent))
}

type codexToolIntent struct {
	File       bool
	Write      bool
	Command    bool
	Web        bool
	Fetch      bool
	Image      bool
	Memory     bool
	Plan       bool
	Status     bool
	CLIHub     bool
	Delegate   bool
	Desktop    bool
	Browser    bool
	Skill      bool
	Automation bool
}

func classifyCodexToolIntent(query string, rawInput string, cliHubIntent bool) codexToolIntent {
	rawLower := strings.ToLower(strings.TrimSpace(rawInput))
	intent := codexToolIntent{CLIHub: cliHubIntent}
	cjkAction := containsCJKActionIntent(query)
	naturalAction := containsNaturalActionIntent(query)

	intent.File = cjkAction || hasAnyToolSelectionKeyword(query, rawLower, "read", "view", "show", "open", "inspect", "file", "folder", "directory", "list", "search", "find", "code", "project", "repo", "workspace", "读取", "查看", "搜索", "查找", "文件", "目录", "项目", "代码")
	intent.Write = cjkAction || naturalAction || hasAnyToolSelectionKeyword(query, rawLower, "write", "edit", "modify", "change", "create", "delete", "remove", "patch", "apply", "fix", "implement", "code", "refactor", "build", "make", "generate", "生成", "创建", "新建", "编辑", "修改", "删除", "修复", "实现", "编写")
	intent.Command = cjkAction || hasAnyToolSelectionKeyword(query, rawLower, "run", "execute", "command", "terminal", "shell", "install", "build", "test", "compile", "start", "运行", "执行", "命令", "终端", "安装", "构建", "测试", "编译", "启动")
	intent.Web = hasAnyToolSelectionKeyword(query, rawLower, "web", "search web", "browse", "research", "latest", "online", "联网", "上网", "搜索网页", "浏览网页", "最新")
	intent.Fetch = strings.Contains(rawLower, "http://") || strings.Contains(rawLower, "https://") || hasAnyToolSelectionKeyword(query, rawLower, "url", "fetch", "download", "website", "page", "网页", "链接", "下载")
	intent.Image = hasAnyToolSelectionKeyword(query, rawLower, "image", "picture", "photo", "screenshot", "vision", "ocr", "图片", "照片", "截图", "识图")
	intent.Memory = hasAnyToolSelectionKeyword(query, rawLower, "memory", "remember", "recall", "history", "记忆", "记住", "回忆")
	intent.Plan = hasAnyToolSelectionKeyword(query, rawLower, "plan", "steps", "todo", "task", "debug", "architecture", "design", "计划", "步骤", "任务", "调试", "架构", "设计")
	intent.Status = hasAnyToolSelectionKeyword(query, rawLower, "session", "status", "context", "状态", "上下文", "会话")
	intent.CLIHub = intent.CLIHub || hasAnyToolSelectionKeyword(query, rawLower, "clihub", "intent", "capability")
	intent.Delegate = hasAnyToolSelectionKeyword(query, rawLower, "delegate", "subagent", "sub agent", "委派", "子代理")
	localPageIntent := containsLocalPageOpenIntent(query, rawLower)
	intent.Browser = strings.Contains(rawLower, "http://") || strings.Contains(rawLower, "https://") || hasAnyToolSelectionKeyword(query, rawLower, "browser", "chrome", "edge", "page", "tab", "click", "type", "浏览器", "网页", "页面", "标签页", "点击", "输入")
	intent.Desktop = localPageIntent || hasAnyToolSelectionKeyword(query, rawLower, "desktop", "window", "app", "application", "screen", "mouse", "keyboard", "open app", "local app", "桌面", "窗口", "应用", "软件", "屏幕", "鼠标", "键盘") || containsDesktopAppIntent(query, rawLower)
	intent.Automation = intent.Desktop || intent.Browser || intent.CLIHub || hasAnyToolSelectionKeyword(query, rawLower, "automate", "workflow", "gui", "自动化", "流程")
	intent.Skill = hasAnyToolSelectionKeyword(query, rawLower, "skill", "技能")

	return intent
}

func selectedCoreToolNamesForIntent(query string, rawInput string, intent codexToolIntent) map[string]struct{} {
	selected := make(map[string]struct{})
	add := func(names ...string) {
		for _, name := range names {
			selected[name] = struct{}{}
		}
	}

	if intent.File || intent.Write {
		add("read_file", "read", "list_directory", "search_files")
	}
	if intent.Write {
		add("write_file", "write", "edit", "apply_patch")
	}
	if intent.Command || intent.Write {
		add("run_command", "exec", "process")
	}
	if intent.Web {
		add("web_search")
	}
	if intent.Fetch {
		add("fetch_url", "web_fetch")
	}
	if intent.Image {
		add("image", "image_analyze")
	}
	if intent.Memory {
		add("memory_search", "memory_get")
	}
	if intent.Plan {
		add("update_plan")
	}
	if intent.Status {
		add("session_status", "claw_bridge_context")
	}
	if intent.CLIHub {
		add("clihub_catalog", "clihub_exec", "intent_route", "intent_list_capabilities")
	}
	if intent.Delegate {
		add("delegate_task")
	}

	return selected
}

func selectedToolPrefixesForIntent(query string, toolList []tools.ToolInfo, intent codexToolIntent) []string {
	prefixes := make([]string, 0, 8)
	if intent.Browser {
		prefixes = append(prefixes, "browser_")
	}
	if intent.Desktop {
		prefixes = append(prefixes, "desktop_", "computer_")
	}
	if intent.Skill {
		prefixes = append(prefixes, "skill_")
	}
	if intent.Memory {
		prefixes = append(prefixes, "memory_")
	}
	if intent.CLIHub {
		prefixes = append(prefixes, "clihub_", "intent_")
	}
	return uniquePrefixes(prefixes)
}

func containsDesktopAppIntent(query string, rawLower string) bool {
	if strings.TrimSpace(query) == "" && strings.TrimSpace(rawLower) == "" {
		return false
	}
	hasLaunchVerb := hasAnyToolSelectionKeyword(query, rawLower, "open", "launch", "start", "打开", "启动")
	if !hasLaunchVerb {
		return false
	}
	if strings.Contains(rawLower, "http://") || strings.Contains(rawLower, "https://") {
		return false
	}
	for _, phrase := range []string{
		"open file", "open folder", "open directory", "open repo", "open repository", "open project", "open url", "open website", "open page",
		"打开文件", "打开文件夹", "打开目录", "打开项目", "打开链接", "打开网页", "打开页面",
	} {
		if strings.Contains(query, phrase) || strings.Contains(rawLower, phrase) {
			return false
		}
	}
	return !hasAnyToolSelectionKeyword(query, rawLower, "browser", "浏览器")
}

func containsLocalPageOpenIntent(query string, rawLower string) bool {
	if strings.Contains(rawLower, "http://") || strings.Contains(rawLower, "https://") {
		return false
	}
	for _, phrase := range []string{
		"open generated page", "open the generated page", "open generated webpage", "open local page", "open local webpage",
		"preview page", "preview webpage", "open html", "open html file", "open the page you generated",
		"打开生成的页面", "打开你生成的页面", "打开刚生成的页面", "打开本地页面", "打开网页文件",
		"预览页面", "预览网页", "打开html", "打开 html", "打开页面",
	} {
		if strings.Contains(query, phrase) || strings.Contains(rawLower, phrase) {
			return true
		}
	}
	return false
}

func uniquePrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(prefixes))
	result := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		result = append(result, prefix)
	}
	return result
}

func hasAnyToolSelectionKeyword(query string, rawLower string, keywords ...string) bool {
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(strings.ToLower(keyword))
		if keyword == "" {
			continue
		}
		if strings.Contains(query, keyword) || strings.Contains(rawLower, keyword) {
			return true
		}
	}
	return false
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
		"打开", "启动", "运行", "执行", "搜索", "查找", "读取", "查看", "编辑", "修改", "创建", "建立", "新建", "删除",
		"发送", "点击", "输入", "截图", "浏览", "访问", "安装", "修复", "实现", "编写", "帮我", "请帮我", "ocr", "memory",
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
		"编辑", "修改", "创建", "建立", "新建", "删除", "发送", "点击",
		"输入", "截图", "浏览", "访问", "安装", "修复", "实现", "编写",
		"帮我", "请帮我",
	}
	for _, term := range actionTerms {
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
		"export", "render", "timeline", "project", "model", "spreadsheet", "presentation", "video", "audio",
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
	case "browser", "computer", "desktop", "skill", "memory", "intent", "clihub", "claw", "read", "write", "list", "search", "run", "web", "fetch":
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

func limitSelectedToolInfos(toolList []tools.ToolInfo, limit int) []tools.ToolInfo {
	if len(toolList) == 0 {
		return nil
	}
	sort.SliceStable(toolList, func(i, j int) bool {
		left := toolPriority(toolList[i].Name)
		right := toolPriority(toolList[j].Name)
		if left != right {
			return left < right
		}
		return toolList[i].Name < toolList[j].Name
	})
	if limit > 0 && len(toolList) > limit {
		toolList = toolList[:limit]
	}
	return append([]tools.ToolInfo(nil), toolList...)
}

func toolPriority(name string) int {
	switch name {
	case "read_file", "read", "list_directory", "search_files":
		return 10
	case "write_file", "write", "edit", "apply_patch":
		return 20
	case "run_command", "exec", "process":
		return 30
	case "fetch_url", "web_fetch", "web_search":
		return 40
	case "image", "image_analyze":
		return 50
	case "update_plan", "session_status":
		return 60
	case "memory_search", "memory_get":
		return 70
	case "intent_route", "intent_list_capabilities", "clihub_catalog", "clihub_exec":
		return 80
	case "computer_observe", "computer_action":
		return 90
	default:
		switch {
		case strings.HasPrefix(name, "desktop_"):
			return 100
		case strings.HasPrefix(name, "browser_"):
			return 110
		case strings.HasPrefix(name, "skill_"):
			return 120
		default:
			return 200
		}
	}
}

func shouldIncludeCLIHubPromptInfo(toolList []tools.ToolInfo) bool {
	for _, tool := range toolList {
		if strings.HasPrefix(tool.Name, "clihub_") || strings.HasPrefix(tool.Name, "intent_") {
			return true
		}
	}
	return false
}

func shouldIncludeClawBridgePromptInfo(toolList []tools.ToolInfo) bool {
	for _, tool := range toolList {
		if tool.Name == "claw_bridge_context" || strings.HasPrefix(tool.Name, "claw_") {
			return true
		}
	}
	return false
}

func (a *Agent) selectedSkillPrompts(toolList []tools.ToolInfo) []string {
	if a == nil || a.skills == nil || len(toolList) == 0 {
		return nil
	}
	selected := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for _, tool := range toolList {
		if !strings.HasPrefix(tool.Name, "skill_") {
			continue
		}
		name := strings.TrimPrefix(tool.Name, "skill_")
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		promptText, err := a.skills.GetPrompt(name, "system")
		if err != nil || strings.TrimSpace(promptText) == "" {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, truncateString(strings.TrimSpace(promptText), 2400))
		if len(selected) >= 4 {
			break
		}
	}
	return selected
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

func (a *Agent) buildSelectedToolDefinitions(toolList []tools.ToolInfo) []llm.ToolDefinition {
	defs := buildToolDefinitionsFromInfos(toolList)
	defs = compactToolDefinitions(defs)
	if a.contextRuntime != nil {
		return a.contextRuntime.limitToolDefinitions(defs)
	}
	return limitToolDefinitionsByBudget(defs, nil, codexSelectedToolDefinitionBudget)
}

func buildToolDefinitionsFromInfos(toolList []tools.ToolInfo) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(toolList))
	for _, t := range toolList {
		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        t.Name,
				Description: truncateString(strings.TrimSpace(t.Description), codexMaxToolDescriptionChars),
				Parameters:  compactToolSchema(t.InputSchema),
			},
		})
	}
	return defs
}

func compactToolDefinitions(defs []llm.ToolDefinition) []llm.ToolDefinition {
	if len(defs) == 0 {
		return nil
	}
	out := make([]llm.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		def.Function.Description = truncateString(strings.TrimSpace(def.Function.Description), codexMaxToolDescriptionChars)
		def.Function.Parameters = compactToolSchema(def.Function.Parameters)
		out = append(out, def)
	}
	return out
}

func compactToolSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	if encoded, err := json.Marshal(schema); err == nil && len(encoded) <= codexMaxToolSchemaBytes {
		return schema
	}
	return compactObjectSchema(schema)
}

func compactObjectSchema(schema map[string]any) map[string]any {
	result := map[string]any{"type": firstNonEmpty(fmt.Sprintf("%v", schema["type"]), "object")}
	if result["type"] == "<nil>" {
		result["type"] = "object"
	}
	if required, ok := schema["required"]; ok {
		result["required"] = required
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return result
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > codexMaxCompactToolSchemaProperties {
		names = names[:codexMaxCompactToolSchemaProperties]
		result["additionalProperties"] = true
	}
	compactProps := make(map[string]any, len(names))
	for _, name := range names {
		compactProps[name] = compactSchemaProperty(properties[name])
	}
	result["properties"] = compactProps
	return result
}

func compactSchemaProperty(value any) any {
	prop, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, 3)
	if typ, ok := prop["type"]; ok {
		out["type"] = typ
	}
	if enum, ok := prop["enum"]; ok {
		out["enum"] = enum
	}
	if desc, ok := prop["description"].(string); ok && strings.TrimSpace(desc) != "" {
		out["description"] = truncateString(strings.TrimSpace(desc), codexMaxCompactToolPropertyDescChars)
	}
	if len(out) == 0 {
		return map[string]any{"type": "string"}
	}
	return out
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
	if a.memory == nil {
		return "", fmt.Errorf("memory backend is nil")
	}
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

func (a *Agent) SetSkills(manager *skills.SkillsManager) {
	a.skills = manager
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

func (a *Agent) prepareSystemPrompt(ctx context.Context, selectedTools []tools.ToolInfo, toolDefs []llm.ToolDefinition, capabilityPlan *CapabilityPlan) (string, error) {
	if a.contextRuntime != nil {
		compacted, err := a.contextRuntime.compactHistory(ctx, a.history, a.llm)
		if err != nil {
			return "", fmt.Errorf("failed to compact history: %w", err)
		}
		a.history = compacted
	}

	systemPrompt, err := a.buildSystemPromptForToolInfosWithPlan(selectedTools, capabilityPlan)
	if err != nil {
		return "", fmt.Errorf("failed to build system prompt: %w", err)
	}

	if a.contextRuntime != nil {
		trimmed, err := a.contextRuntime.enforceTokenLimit(systemPrompt, a.history, toolDefs)
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
