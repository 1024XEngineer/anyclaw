package prompt

import (
	"encoding/xml"
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
	if identity := b.buildIdentity(data); identity != "" {
		parts = append(parts, identity)
	}
	if capabilities := b.buildCapabilities(data); capabilities != "" {
		parts = append(parts, capabilities)
	}
	if capabilityPlan := b.buildCapabilityPlanning(data); capabilityPlan != "" {
		parts = append(parts, capabilityPlan)
	}
	if operatingMode := b.buildOperatingMode(data); operatingMode != "" {
		parts = append(parts, operatingMode)
	}
	if cliHub := b.buildCLIHub(data); cliHub != "" {
		parts = append(parts, cliHub)
	}
	if clawBridge := b.buildClawBridge(data); clawBridge != "" {
		parts = append(parts, clawBridge)
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
	if skills := b.buildSkills(data); skills != "" {
		parts = append(parts, skills)
	}
	parts = append(parts, b.buildGuidelines())
	parts = append(parts, b.buildInstructions())
	parts = append(parts, b.buildResponseStyle())

	return strings.Join(parts, "\n\n"), nil
}

func (b *SystemPromptBuilder) buildHeader() string {
	return `You are AnyClaw, a Codex-style coding and local execution agent. You and the user share one workspace, and your job is to collaborate with them until their goal is genuinely handled.`
}

func (b *SystemPromptBuilder) buildIdentity(data PromptData) string {
	var parts []string
	parts = append(parts, "## Identity")
	if b.name != "" {
		parts = append(parts, fmt.Sprintf("- Name: %s", b.name))
	}
	if b.description != "" {
		parts = append(parts, fmt.Sprintf("- Description: %s", b.description))
	}
	if strings.TrimSpace(data.SystemPrompt) != "" {
		parts = append(parts, "- Primary system prompt:")
		parts = append(parts, strings.TrimSpace(data.SystemPrompt))
	}
	if strings.TrimSpace(data.Personality) != "" {
		parts = append(parts, "- Personality supplement:")
		parts = append(parts, strings.TrimSpace(data.Personality))
	}
	parts = append(parts, "- Work like a senior engineer in the user's workspace: read the existing context first, make conservative changes that fit the codebase, verify the result, and keep going until the task is complete or clearly blocked.")
	return strings.Join(parts, "\n")
}

func (b *SystemPromptBuilder) buildCapabilities(data PromptData) string {
	parts := []string{
		"## Capabilities",
		"You can inspect files, run commands, browse the web, and interact with local apps through structured tools.",
	}

	if len(data.Tools) == 0 {
		parts = append(parts, "- No tools are selected for this turn.")
		return strings.Join(parts, "\n")
	}

	if len(data.Tools) <= 12 {
		parts = append(parts, "Tools available right now:")
		for _, tool := range data.Tools {
			parts = append(parts, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
		}
		return strings.Join(parts, "\n")
	}

	parts = append(parts, fmt.Sprintf("- %d tools are selected for this turn.", len(data.Tools)))
	if families := summarizeToolFamilies(data.Tools, 8); len(families) > 0 {
		parts = append(parts, "- Relevant tool families: "+strings.Join(families, ", ")+".")
	}
	parts = append(parts, "Representative tools:")
	for _, tool := range data.Tools[:minInt(len(data.Tools), 12)] {
		parts = append(parts, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}

	return strings.Join(parts, "\n")
}

func (b *SystemPromptBuilder) buildCapabilityPlanning(data PromptData) string {
	plan := data.CapabilityPlan
	if strings.TrimSpace(plan.TaskClass) == "" || strings.TrimSpace(plan.Route) == "" {
		return ""
	}

	lines := []string{
		"## Capability Planning",
		fmt.Sprintf("- Task class: %s.", plan.TaskClass),
		fmt.Sprintf("- Planned route: %s.", plan.Route),
	}
	if strings.TrimSpace(plan.Need) != "" {
		lines = append(lines, "- Capability need: "+trimPromptLine(plan.Need, 180)+".")
	}
	if strings.TrimSpace(plan.KindHint) != "" {
		lines = append(lines, "- Preferred capability kind: "+trimPromptLine(plan.KindHint, 80)+".")
	}
	if strings.TrimSpace(plan.TopLocalMatch.Name) != "" {
		match := plan.TopLocalMatch
		matchLine := fmt.Sprintf("- Best local match: %s/%s", trimPromptLine(match.Kind, 60), trimPromptLine(match.Name, 100))
		if match.Score > 0 {
			matchLine += fmt.Sprintf(" (score %.2f)", match.Score)
		}
		if strings.TrimSpace(match.Reason) != "" {
			matchLine += ": " + trimPromptLine(match.Reason, 180)
		}
		lines = append(lines, matchLine+".")
	}
	if strings.TrimSpace(plan.Reason) != "" {
		lines = append(lines, "- Reason: "+trimPromptLine(plan.Reason, 220)+".")
	}
	if plan.ShouldExposeMarketSearch {
		lines = append(lines,
			"- If local tools and skills are insufficient, use market_search_artifacts to search installed and cloud marketplace artifacts for this missing capability.",
			"- Do not call market_install_artifact or market_bind_artifact unless the user explicitly confirms the proposed artifact and any required risk acknowledgement.",
		)
	}
	if hasTool(data.Tools, "market_install_artifact") || hasTool(data.Tools, "market_bind_artifact") {
		lines = append(lines,
			"- Marketplace install/bind tools are visible because this turn appears to be a follow-up confirmation. Use the artifact id from the conversation or ask for clarification if multiple candidates are possible.",
			"- Call market_install_artifact only after explicit user confirmation. If it returns requires_confirmation, explain the policy decision and ask for the missing confirmation or risk acknowledgement instead of retrying blindly.",
			"- Call market_bind_artifact only after an artifact is installed and the user has confirmed where it should be bound. After install or bind succeeds, continue the original task using the refreshed capability when it is available.",
		)
	}
	return strings.Join(lines, "\n")
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
	ComputerUseTools   []ToolInfo
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
		"## Operating Mode",
		"You are Codex-style: a completion-oriented agent for coding, debugging, local execution, and real-world tasks. Your job is to get the task done safely on this machine, not merely describe how it could be done.",
		"Execution contract:",
		"- Treat tool outputs as evidence about the current world state.",
		"- Do not guess the state of files, commands, webpages, windows, or apps when you can inspect them.",
		"- Work in loops: inspect the current state, choose the next best action, execute it, inspect again, and adapt until the requested outcome is complete.",
		"- For coding tasks, read the codebase first and follow existing patterns before editing.",
		"- Prefer ripgrep-style fast search, standard libraries, and mature maintained libraries over ad hoc implementations when they fit the project.",
		"- Keep edits scoped. Do not revert or overwrite user changes unless the user explicitly asks you to.",
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
		lines = append(lines, "- When the user explicitly wants a visible browser window or asks to open a URL on the desktop, prefer desktop_open instead of browser_navigate.")
		hasSpecificOrder = true
	}

	if len(catalog.DesktopTarget) > 0 {
		lines = append(lines, fmt.Sprintf("3. For local apps, prefer target-based desktop tools instead of raw coordinates: %s.", formatToolNameList(catalog.DesktopTarget, 6)))
		lines = append(lines, "- Inside target-based desktop work, prefer stable selectors first: UI automation, then visible text/OCR, then image matching, then window-only fallback.")
		hasSpecificOrder = true
	}

	if len(catalog.ComputerUseTools) > 0 {
		lines = append(lines, fmt.Sprintf("4. For visible desktop work that needs screen-level control, use computer_observe and small computer_action batches before falling back to raw desktop tools: %s.", formatToolNameList(catalog.ComputerUseTools, 4)))
		lines = append(lines, "- Use normalized 0-1000 coordinates for computer actions unless absolute coordinates are explicitly required, and inspect the returned observation after each meaningful action batch.")
		hasSpecificOrder = true
	}

	if len(catalog.DesktopLowLevel) > 0 {
		lines = append(lines, fmt.Sprintf("5. Use low-level desktop tools only as a fallback when target-based or computer-use tools cannot complete the step reliably: %s.", formatToolNameList(catalog.DesktopLowLevel, 8)))
		hasSpecificOrder = true
	}

	if len(catalog.DesktopObservation) > 0 {
		lines = append(lines, fmt.Sprintf("6. After important actions, verify the result with observation tools instead of assuming success: %s.", formatToolNameList(catalog.DesktopObservation, 8)))
		hasSpecificOrder = true
	} else {
		lines = append(lines, "6. After important actions, check whether the requested outcome actually appeared before moving on.")
		hasSpecificOrder = true
	}

	if !hasSpecificOrder {
		lines = append(lines, "1. Start from the available evidence, make the next necessary change, then inspect again before deciding the task is done.")
	}

	lines = append(lines,
		"Approval and recovery:",
		"- When approvals are required, prepare a concise action plan, request approval, and resume execution after approval instead of abandoning the task.",
		"- If the user already explicitly requested a local action and the tool approval prompt will capture consent, do not ask an extra conversational confirmation unless some detail is genuinely ambiguous.",
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
	return fmt.Sprintf("## Workspace\nWorking directory: %s", data.WorkingDir)
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
Durable memory is stored in Codex-style runtime storage, scoped to this workspace, not in the project workspace.
Do not assume older context is present in the prompt. Use memory_search or memory_get only when older context is needed.`
}

func (b *SystemPromptBuilder) buildSkills(data PromptData) string {
	if len(data.AvailableSkills) > 0 {
		return b.buildAvailableSkills(data.AvailableSkills)
	}
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

func (b *SystemPromptBuilder) buildAvailableSkills(skills []AvailableSkill) string {
	lines := []string{
		"## Skills (mandatory)",
		"Before replying: scan <available_skills> <description> entries.",
		"- If exactly one skill clearly applies: read its file at <location> with `read`, then follow it. You MUST use the exact <location> value from <available_skills>; never guess, fabricate, or hard-code a skill file path.",
		"- If multiple could apply: choose the most specific one, read its file at <location> with `read`, then follow it. You MUST use the exact <location> value from <available_skills>; never guess, fabricate, or hard-code a skill file path.",
		"- If none clearly apply: do not read any skill file.",
		"Constraints: never read more than one skill up front; only read after selecting.",
		"- When a skill drives external API writes, assume rate limits: prefer fewer larger writes, avoid tight one-item loops, serialize bursts when possible, and respect 429/Retry-After.",
		"The following skills provide specialized instructions for specific tasks.",
		"Use the read tool to load a skill's file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"<available_skills>",
	}

	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		location := strings.TrimSpace(skill.Location)
		if name == "" || location == "" {
			continue
		}
		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = "Specialized AnyClaw skill."
		}
		lines = append(lines,
			"  <skill>",
			"    <name>"+escapeXMLText(name)+"</name>",
			"    <description>"+escapeXMLText(description)+"</description>",
			"    <location>"+escapeXMLText(location)+"</location>",
			"  </skill>",
		)
	}
	lines = append(lines, "</available_skills>")
	return strings.Join(lines, "\n")
}

func (b *SystemPromptBuilder) buildGuidelines() string {
	return `## Guidelines
- Be helpful, honest, and completion-oriented.
- Read the codebase and local instructions before making nontrivial code changes.
- Prefer the repository's existing patterns, frameworks, helper APIs, and tests.
- Prefer observable evidence over guesses.
- When the user wants a task completed and the tools allow it, take action instead of stopping at advice.
- Keep progress claims tied to inspected state.
- If a result is partially done but not yet verified, keep working or clearly state the remaining uncertainty.
- Keep project-specific instructions in AGENTS.md; do not create separate workspace memory files.`
}

func (b *SystemPromptBuilder) buildInstructions() string {
	return `## Instructions
- Always respond in the same language as the user.
- Unless the user only asks a question, asks for a plan, or clearly says not to edit, assume they want you to make the change or run the needed tools.
- Prefer native structured tool calls when tools are available.
- Before editing files, understand the surrounding code and choose a conservative implementation.
- Use comments sparingly and only when they clarify non-obvious logic.
- For code review requests, lead with findings ordered by severity and include file/line references when possible.
- Keep updates concise and focused on what changed, what was verified, and what remains blocked or unverified.
- Do not say a task is complete unless the requested outcome has been checked against observable evidence or you explicitly state what could not be verified.`
}

func (b *SystemPromptBuilder) buildResponseStyle() string {
	return `## Final Response Style
- Report results like Codex: concise, direct, and grounded in what actually changed or was verified.
- For task execution, first tell the user the task is done or what was completed in a natural sentence. Then provide expandable/detail-friendly evidence.
- In Chinese, prefer this compact shape when actions were taken:
完成了：...

已处理：
- ...
已验证：
- ...
未确认/阻塞：
- 无
- If there were no tool actions and the user asked a normal question, answer naturally without forcing this structure.
- Do not include raw tool logs unless the user asks for them; summarize the relevant evidence instead.`
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

func escapeXMLText(value string) string {
	var builder strings.Builder
	if err := xml.EscapeText(&builder, []byte(value)); err != nil {
		return value
	}
	return builder.String()
}

type PromptData struct {
	Name            string
	Description     string
	SystemPrompt    string
	Personality     string
	WorkingDir      string
	Skills          []string
	SkillPrompts    []string
	AvailableSkills []AvailableSkill
	CapabilityPlan  CapabilityPlanInfo
	Tools           []ToolInfo
	CLIHub          *CLIHubInfo
	ClawBridge      *ClawBridgeInfo
	WorkspaceFiles  []WorkspaceFile
	History         []Message
}

type CapabilityPlanInfo struct {
	TaskClass                string
	Route                    string
	Need                     string
	KindHint                 string
	TopLocalMatch            CapabilityMatchInfo
	ShouldExposeMarketSearch bool
	Reason                   string
}

type CapabilityMatchInfo struct {
	Kind   string
	Name   string
	Score  float64
	Reason string
}

type AvailableSkill struct {
	Name        string
	Description string
	Location    string
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
		case strings.HasPrefix(name, "computer_"):
			catalog.ComputerUseTools = append(catalog.ComputerUseTools, tool)
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
	return len(c.FileStateTools) > 0 || len(c.CommandTools) > 0 || len(c.AppWorkflowTools) > 0 || len(c.AppActionTools) > 0 || len(c.BrowserTools) > 0 || len(c.ComputerUseTools) > 0 || len(c.DesktopTarget) > 0 || len(c.DesktopObservation) > 0 || len(c.DesktopLowLevel) > 0
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

func trimPromptLine(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 1 {
		return value[:limit]
	}
	return value[:limit-1] + "..."
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
