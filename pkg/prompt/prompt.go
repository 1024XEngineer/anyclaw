package prompt

import (
	"fmt"
	"strings"
	"text/template"
)

type SystemPromptBuilder struct {
	name        string
	description string
	templates   map[string]*template.Template
}

func NewSystemPromptBuilder(name, description string) *SystemPromptBuilder {
	return &SystemPromptBuilder{
		name:        name,
		description: description,
		templates:   make(map[string]*template.Template),
	}
}

func (b *SystemPromptBuilder) RegisterTemplate(name string, tmpl string) error {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return err
	}
	b.templates[name] = t
	return nil
}

func (b *SystemPromptBuilder) Build(data PromptData) (string, error) {
	var parts []string

	parts = append(parts, b.buildHeader())
	parts = append(parts, b.buildIdentity())
	parts = append(parts, b.buildCapabilities(data))
	if cliHub := b.buildCLIHub(data); cliHub != "" {
		parts = append(parts, cliHub)
	}
	if clawBridge := b.buildClawBridge(data); clawBridge != "" {
		parts = append(parts, clawBridge)
	}
	if operatingMode := b.buildOperatingMode(data); operatingMode != "" {
		parts = append(parts, operatingMode)
	}
	if workspace := b.buildWorkspace(data); workspace != "" {
		parts = append(parts, workspace)
	}
	if workspaceFiles := b.buildWorkspaceFiles(data); workspaceFiles != "" {
		parts = append(parts, workspaceFiles)
	}
	if memoryRecall := b.buildMemoryRecall(data); memoryRecall != "" {
		parts = append(parts, memoryRecall)
	}
	parts = append(parts, b.buildMemory(data))
	parts = append(parts, b.buildSkills(data))
	parts = append(parts, b.buildGuidelines())
	parts = append(parts, b.buildInstructions())

	return strings.Join(parts, "\n\n"), nil
}

func (b *SystemPromptBuilder) buildHeader() string {
	return `You are AnyClaw, a local-first execution agent focused on safely completing real tasks instead of only answering about them.`
}

func (b *SystemPromptBuilder) buildIdentity() string {
	var parts []string

	if b.name != "" {
		parts = append(parts, fmt.Sprintf("Your name is %s.", b.name))
	}
	if b.description != "" {
		parts = append(parts, b.description)
	}
	parts = append(parts, "You have a configurable personality profile. Follow the tone, style, constraints, and operating traits provided in your identity description.")
	parts = append(parts, "Operate like a careful human teammate on the local machine: move the task forward, observe what changed, and adapt until the deliverable is actually complete or clearly blocked.")

	return strings.Join(parts, " ")
}

func (b *SystemPromptBuilder) buildCapabilities(data PromptData) string {
	parts := []string{
		"You can use structured tools when the task requires inspecting files, running commands, browsing the web, or interacting with local apps.",
	}

	if len(data.Tools) == 0 {
		parts = append(parts, "(No tools selected for this turn)")
		return strings.Join(parts, "\n")
	}

	if len(data.Tools) <= 12 {
		parts = append(parts, "Tools available right now:")
		for _, tool := range data.Tools {
			parts = append(parts, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
		}
		return strings.Join(parts, "\n")
	}

	parts = append(parts, fmt.Sprintf("There are %d tools selected for this turn.", len(data.Tools)))
	if families := summarizeToolFamilies(data.Tools, 8); len(families) > 0 {
		parts = append(parts, "Relevant tool families: "+strings.Join(families, ", ")+".")
	}
	parts = append(parts, "Representative tools:")
	for _, tool := range data.Tools[:minInt(len(data.Tools), 12)] {
		parts = append(parts, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}

	return strings.Join(parts, "\n")
}

func (b *SystemPromptBuilder) buildClawBridge(data PromptData) string {
	if data.ClawBridge == nil {
		return ""
	}

	lines := []string{
		"## Claw Bridge",
		fmt.Sprintf("A local claw-code-main reference surface is available at: %s", data.ClawBridge.Root),
		fmt.Sprintf("- Snapshot size: %d command entries, %d tool entries, %d mirrored subsystems.", data.ClawBridge.CommandsCount, data.ClawBridge.ToolsCount, data.ClawBridge.SubsystemsCount),
		"- Use it as a thinking aid for end-to-end execution, not as a requirement to mimic every archived name literally.",
		"- Favor an execution loop like: inspect -> plan -> execute -> verify -> review -> adapt.",
	}

	if len(data.ClawBridge.CommandFamilies) > 0 {
		lines = append(lines, "- Command domains to think with: "+strings.Join(data.ClawBridge.CommandFamilies, ", ")+".")
	}
	if len(data.ClawBridge.ToolFamilies) > 0 {
		lines = append(lines, "- Tool families worth remembering: "+strings.Join(data.ClawBridge.ToolFamilies, ", ")+".")
	}
	if len(data.ClawBridge.Subsystems) > 0 {
		lines = append(lines, "- High-signal subsystems: "+strings.Join(data.ClawBridge.Subsystems, ", ")+".")
	}

	return strings.Join(lines, "\n")
}

func (b *SystemPromptBuilder) buildCLIHub(data PromptData) string {
	if data.CLIHub == nil {
		return ""
	}

	lines := []string{
		"## CLI Hub",
		fmt.Sprintf("A local CLI-Anything catalog is available at: %s", data.CLIHub.Root),
		fmt.Sprintf("- Catalog size: %d harness entries across %d categories.", data.CLIHub.EntriesCount, len(data.CLIHub.Categories)),
		fmt.Sprintf("- Runnable harnesses visible from this runtime: %d.", data.CLIHub.RunnableCount),
		fmt.Sprintf("- Installed harnesses visible from this runtime: %d.", data.CLIHub.InstalledCount),
		"- When a task maps well to a known harness, prefer clihub_catalog to inspect options and clihub_exec to run the harness instead of composing raw shell manually.",
		"- CLI-Anything harnesses are designed for agent use: structured commands, JSON output, stateful workflows, and real backend integration.",
	}
	if data.CLIHub.CapabilitiesCount > 0 {
		lines = append(lines, fmt.Sprintf("- Auto-routable capabilities indexed: %d.", data.CLIHub.CapabilitiesCount))
	}
	if hasTool(data.Tools, "intent_route") {
		lines = append(lines, "- For concrete software actions stated in natural language, prefer intent_route first so the agent can auto-pick the right harness and execute it directly.")
	}
	if hasTool(data.Tools, "intent_list_capabilities") {
		lines = append(lines, "- If the intent is ambiguous, call intent_list_capabilities with the user's wording to inspect the best matching capabilities before falling back to clihub_exec.")
	}
	if len(data.CLIHub.Categories) > 0 {
		lines = append(lines, "- Strong categories in this catalog: "+strings.Join(data.CLIHub.Categories, ", ")+".")
	}
	if len(data.CLIHub.Runnable) > 0 {
		lines = append(lines, "- Harnesses ready right now (installed or local source): "+strings.Join(data.CLIHub.Runnable, ", ")+".")
	}
	if len(data.CLIHub.Installed) > 0 {
		lines = append(lines, "- Installed harnesses ready right now: "+strings.Join(data.CLIHub.Installed, ", ")+".")
	}
	if len(data.CLIHub.SkillCommands) > 0 {
		lines = append(lines, "- Available direct skill commands: "+strings.Join(data.CLIHub.SkillCommands, ", ")+".")
		lines = append(lines, "- Use these direct skill tools instead of clihub_exec when available.")
	}
	if len(data.CLIHub.IntentExamples) > 0 {
		lines = append(lines, "- Example routed capabilities from this catalog: "+strings.Join(data.CLIHub.IntentExamples, ", ")+".")
	}

	return strings.Join(lines, "\n")
}

type executionToolCatalog struct {
	AppWorkflowTools   []ToolInfo
	AppActionTools     []ToolInfo
	BrowserTools       []ToolInfo
	BrowserObservation []ToolInfo
	DesktopTarget      []ToolInfo
	DesktopObservation []ToolInfo
	DesktopLowLevel    []ToolInfo
	FileStateTools     []ToolInfo
	CommandTools       []ToolInfo
}

func (b *SystemPromptBuilder) buildOperatingMode(data PromptData) string {
	catalog := classifyExecutionTools(data.Tools)
	if !catalog.HasCoreExecutionTools() {
		return ""
	}

	lines := []string{
		"## AnyClaw Core",
		"You are a human-like local execution agent. Your primary job is to complete the user's task safely on this machine, not merely explain how it could be done.",
		"If a person could safely do the task locally without harming the computer or exposing protected/private data, prefer completing it.",
		"Execution contract:",
		"- Treat tool outputs as evidence about the current world state.",
		"- Do not guess the state of files, commands, webpages, windows, or apps when you can inspect them.",
		"- Work in loops: inspect the current state, choose the next best action, execute it, inspect again, and adapt until the requested outcome is complete.",
		"- Before declaring success, verify the requested deliverable with observable evidence. If any part remains unverified, say exactly what is done and what is still unconfirmed.",
	}

	observationLines := buildObservationLines(catalog)
	if len(observationLines) > 0 {
		lines = append(lines, "Current-state observation sources:")
		lines = append(lines, observationLines...)
	}

	lines = append(lines, "Execution order:")
	hasSpecificOrder := false

	if len(catalog.AppWorkflowTools) > 0 {
		lines = append(lines, fmt.Sprintf("1. Prefer higher-level app workflow tools first when they fit the goal: %s.", formatToolNameList(catalog.AppWorkflowTools, 8)))
		hasSpecificOrder = true
	} else if len(catalog.AppActionTools) > 0 {
		lines = append(lines, fmt.Sprintf("1. Prefer higher-level app connector tools before low-level desktop actions: %s.", formatToolNameList(catalog.AppActionTools, 8)))
		hasSpecificOrder = true
	}

	if len(catalog.BrowserTools) > 0 {
		lines = append(lines, fmt.Sprintf("2. For web apps, prefer browser tools over desktop clicking when the same task can be completed in the browser: %s.", formatToolNameList(catalog.BrowserTools, 6)))
		hasSpecificOrder = true
	}

	if len(catalog.DesktopTarget) > 0 {
		lines = append(lines, fmt.Sprintf("3. For local apps, prefer target-based desktop tools instead of raw coordinates: %s.", formatToolNameList(catalog.DesktopTarget, 6)))
		lines = append(lines, "Inside target-based desktop work, prefer stable selectors first: UI automation, then visible text/OCR, then image matching, then window-only fallback.")
		hasSpecificOrder = true
	}

	if len(catalog.DesktopLowLevel) > 0 {
		lines = append(lines, fmt.Sprintf("4. Use low-level desktop tools only as a fallback when target-based tools cannot complete the step reliably: %s.", formatToolNameList(catalog.DesktopLowLevel, 8)))
		hasSpecificOrder = true
	}

	if len(catalog.DesktopObservation) > 0 {
		lines = append(lines, fmt.Sprintf("5. After important actions, verify the result with observation tools instead of assuming success: %s.", formatToolNameList(catalog.DesktopObservation, 8)))
		hasSpecificOrder = true
	} else {
		lines = append(lines, "5. After important actions, check whether the requested outcome actually appeared before moving on.")
		hasSpecificOrder = true
	}

	if !hasSpecificOrder {
		lines = append(lines, "1. Start from the available evidence, make the next necessary change, then inspect again before deciding the task is done.")
	}

	lines = append(lines,
		"Approval and recovery:",
		"- When approvals are required, prepare a concise action plan, request approval, and resume execution after approval instead of abandoning the task.",
		"- If a step fails or the observed state does not match the goal, retry with a more reliable selector, a higher-level workflow, or a different tactic before falling back to raw mouse and keyboard actions.",
		"- When a task needs a multi-step local app procedure, you may emit a structured anyclaw.app.desktop.v1 plan instead of narrating raw step-by-step instructions.",
		"- Prefer completing the requested deliverable and reporting what changed, what was verified, and what remains blocked or unverified.",
	)

	return strings.Join(lines, "\n")
}

func (b *SystemPromptBuilder) buildWorkspace(data PromptData) string {
	if strings.TrimSpace(data.WorkingDir) == "" {
		return ""
	}
	return fmt.Sprintf(`## Workspace
Working directory: %s`, data.WorkingDir)
}

func (b *SystemPromptBuilder) buildWorkspaceFiles(data PromptData) string {
	if len(data.WorkspaceFiles) == 0 {
		return ""
	}

	parts := []string{"## Project Context"}
	for _, file := range data.WorkspaceFiles {
		name := strings.TrimSpace(file.Name)
		if name == "" {
			continue
		}
		content := strings.TrimSpace(file.Content)
		if content == "" {
			content = "(empty)"
		}
		parts = append(parts, fmt.Sprintf("### %s\n%s", name, content))
	}
	return strings.Join(parts, "\n\n")
}

func (b *SystemPromptBuilder) buildMemoryRecall(data PromptData) string {
	if !hasTool(data.Tools, "memory_search") && !hasTool(data.Tools, "memory_get") {
		return ""
	}
	return `## Memory Recall
Daily memory files under workspace/memory are not injected automatically.
Use memory_search to find relevant days and memory_get to open a specific daily memory file when older context is needed.`
}

func (b *SystemPromptBuilder) buildMemory(data PromptData) string {
	if data.Memory == "" {
		return ""
	}

	return fmt.Sprintf(`## Memory
%s`, data.Memory)
}

func (b *SystemPromptBuilder) buildSkills(data PromptData) string {
	if len(data.SkillPrompts) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, "## Active Skills")

	for _, prompt := range data.SkillPrompts {
		parts = append(parts, prompt)
	}

	return strings.Join(parts, "\n\n")
}

func (b *SystemPromptBuilder) buildGuidelines() string {
	return `## Guidelines
- Be helpful, harmless, honest, and completion-oriented
- Think step by step using observable evidence
- When using tools, explain what you're doing
- If a tool fails, explain the error and suggest alternatives
- When tools are available and the user wants a task completed, take action instead of stopping at advice
- Base progress claims on inspected state, not assumptions
- If the task is partly done but not yet verified, keep working or clearly mark the remaining uncertainty
- Store important information in memory for future reference`
}

func (b *SystemPromptBuilder) buildInstructions() string {
	return `## Instructions
- Always respond in the same language as the user
- If tools are available, prefer native structured tool calls; only fall back to textual tool_call JSON if the model cannot emit native tool calls
- After completing a task, summarize what was done, what was verified, and any remaining blocked or unverified part
- Do not say a task is complete unless the requested outcome has been checked against observable evidence or you explicitly state what could not be verified`
}

func summarizeToolFamilies(tools []ToolInfo, limit int) []string {
	if len(tools) == 0 || limit <= 0 {
		return nil
	}
	families := make([]string, 0, limit)
	seen := make(map[string]struct{})
	for _, tool := range tools {
		family := toolFamilyName(tool.Name)
		if family == "" {
			continue
		}
		if _, ok := seen[family]; ok {
			continue
		}
		seen[family] = struct{}{}
		families = append(families, family)
		if len(families) >= limit {
			break
		}
	}
	return families
}

func toolFamilyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if idx := strings.Index(name, "_"); idx > 0 {
		return name[:idx]
	}
	return name
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type PromptData struct {
	Name           string
	Description    string
	WorkingDir     string
	Memory         string
	Skills         []string
	SkillPrompts   []string
	Tools          []ToolInfo
	CLIHub         *CLIHubInfo
	ClawBridge     *ClawBridgeInfo
	WorkspaceFiles []WorkspaceFile
	History        []Message
}

type WorkspaceFile struct {
	Name    string
	Content string
}

type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type CLIHubInfo struct {
	Root              string
	EntriesCount      int
	RunnableCount     int
	InstalledCount    int
	CapabilitiesCount int
	Categories        []string
	Runnable          []string
	Installed         []string
	SkillCommands     []string
	IntentExamples    []string
}

type ClawBridgeInfo struct {
	Root            string
	CommandsCount   int
	ToolsCount      int
	SubsystemsCount int
	CommandFamilies []string
	ToolFamilies    []string
	Subsystems      []string
}

type Message struct {
	Role    string
	Content string
}

func BuildSystemPrompt(name, description string, data PromptData) (string, error) {
	if strings.TrimSpace(data.Description) != "" {
		description = data.Description
	}
	b := NewSystemPromptBuilder(name, description)
	return b.Build(data)
}

func classifyExecutionTools(tools []ToolInfo) executionToolCatalog {
	catalog := executionToolCatalog{}
	for _, tool := range tools {
		name := strings.TrimSpace(strings.ToLower(tool.Name))
		switch {
		case name == "read_file" || name == "list_directory" || name == "search_files":
			catalog.FileStateTools = append(catalog.FileStateTools, tool)
		case name == "run_command":
			catalog.CommandTools = append(catalog.CommandTools, tool)
		case strings.HasPrefix(name, "app_") && strings.Contains(name, "_workflow_"):
			catalog.AppWorkflowTools = append(catalog.AppWorkflowTools, tool)
		case strings.HasPrefix(name, "app_"):
			catalog.AppActionTools = append(catalog.AppActionTools, tool)
		case name == "browser_snapshot" || name == "browser_eval" || name == "browser_screenshot" || name == "browser_wait" || name == "browser_tab_list":
			catalog.BrowserObservation = append(catalog.BrowserObservation, tool)
			catalog.BrowserTools = append(catalog.BrowserTools, tool)
		case strings.HasPrefix(name, "browser_"):
			catalog.BrowserTools = append(catalog.BrowserTools, tool)
		case name == "desktop_resolve_target" || name == "desktop_activate_target" || name == "desktop_set_target_value":
			catalog.DesktopTarget = append(catalog.DesktopTarget, tool)
		case name == "desktop_wait_text" || name == "desktop_wait_image" || name == "desktop_verify_text" || name == "desktop_ocr" || name == "desktop_find_text" || name == "desktop_match_image" || name == "desktop_list_windows" || name == "desktop_wait_window" || name == "desktop_inspect_ui":
			catalog.DesktopObservation = append(catalog.DesktopObservation, tool)
		case strings.HasPrefix(name, "desktop_"):
			catalog.DesktopLowLevel = append(catalog.DesktopLowLevel, tool)
		}
	}
	return catalog
}

func (c executionToolCatalog) HasCoreExecutionTools() bool {
	return len(c.FileStateTools) > 0 || len(c.CommandTools) > 0 || len(c.AppWorkflowTools) > 0 || len(c.AppActionTools) > 0 || len(c.BrowserTools) > 0 || len(c.DesktopTarget) > 0 || len(c.DesktopObservation) > 0 || len(c.DesktopLowLevel) > 0
}

func formatToolNameList(tools []ToolInfo, limit int) string {
	if len(tools) == 0 {
		return ""
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != "" {
			names = append(names, tool.Name)
		}
	}
	if limit > 0 && len(names) > limit {
		remaining := len(names) - limit
		names = append(names[:limit], fmt.Sprintf("and %d more", remaining))
	}
	return strings.Join(names, ", ")
}

func hasTool(tools []ToolInfo, name string) bool {
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.Name), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func buildObservationLines(catalog executionToolCatalog) []string {
	lines := []string{}
	if len(catalog.FileStateTools) > 0 {
		lines = append(lines, fmt.Sprintf("- Workspace and file state: %s.", formatToolNameList(catalog.FileStateTools, 6)))
	}
	if len(catalog.CommandTools) > 0 {
		lines = append(lines, fmt.Sprintf("- Command and system output: %s.", formatToolNameList(catalog.CommandTools, 4)))
	}
	if len(catalog.BrowserObservation) > 0 {
		lines = append(lines, fmt.Sprintf("- Browser and page state: %s.", formatToolNameList(catalog.BrowserObservation, 6)))
	}
	if len(catalog.AppWorkflowTools) > 0 || len(catalog.AppActionTools) > 0 {
		lines = append(lines, fmt.Sprintf("- App-specific actions and state via connectors: %s.", formatToolNameList(append(append([]ToolInfo{}, catalog.AppWorkflowTools...), catalog.AppActionTools...), 8)))
	}
	if len(catalog.DesktopObservation) > 0 {
		lines = append(lines, fmt.Sprintf("- Desktop UI, OCR, screenshots, and window state: %s.", formatToolNameList(catalog.DesktopObservation, 8)))
	}
	return lines
}

func BuildConversationPrompt(messages []Message, systemPrompt string) []map[string]string {
	var result []map[string]string

	if systemPrompt != "" {
		result = append(result, map[string]string{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	for _, msg := range messages {
		result = append(result, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	return result
}

func FormatToolCall(toolName string, args map[string]any) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool: %s\n", toolName))
	sb.WriteString("Arguments:\n")
	for k, v := range args {
		sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
	}
	return sb.String(), nil
}
