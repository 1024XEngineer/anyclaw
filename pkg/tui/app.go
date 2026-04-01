package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type AgentHandler interface {
	Run(ctx context.Context, message string) (string, error)
	RunStream(ctx context.Context, message string, onChunk func(string)) error
	GetHistory() []Message
	ClearHistory()
	SetHistory([]Message)
	ListSkills() []SkillInfo
	ListTools() []ToolInfo
	ShowMemory() (string, error)
}

type SkillsHandler interface {
	List() []SkillInfo
}

type SkillInfo struct {
	Name        string
	Description string
}

type ToolInfo struct {
	Name        string
	Description string
}

type LLMHandler interface {
	GetProvider() string
	GetModel() string
	GetTemperature() float64
	SwitchProvider(provider string) error
	SwitchModel(model string) error
	SetTemperature(temp float64)
}

type Model struct {
	chatLog       ChatLog
	editor        Editor
	header        string
	footer        string
	statusMessage string
	spinner       spinner.Model
	loading       bool
	width         int
	height        int
	showOverlay   bool
	overlay       Overlay

	agent         AgentHandler
	skills        SkillsHandler
	llm           LLMHandler
	agentName     string
	thinkingLevel string
	verboseLevel  string

	startTime    time.Time
	lastResponse string
	tokenCount   int

	onQuit     func() error
	session    *SessionStore
	currentSes *Session

	helpPages       []string
	currentHelpPage int
}

type OverlayType int

const (
	OverlayNone OverlayType = iota
	OverlayModel
	OverlayProvider
	OverlayAgent
	OverlaySession
	OverlaySettings
)

type Overlay struct {
	overlayType OverlayType
	items       []SelectItem
	selectedIdx int
	title       string
	onSelect    func(item SelectItem)
	onCancel    func()
}

type SelectItem struct {
	Label string
	Value string
	Desc  string
}

func NewModel(
	agent AgentHandler,
	skills SkillsHandler,
	llm LLMHandler,
	onQuit func() error,
	workDir string,
) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle

	store := NewSessionStore(workDir)
	store.Init()

	currentSes := store.CreateSession()

	return &Model{
		chatLog:    NewChatLog(),
		editor:     NewEditor(),
		spinner:    s,
		loading:    false,
		agent:      agent,
		skills:     skills,
		llm:        llm,
		onQuit:     onQuit,
		session:    store,
		currentSes: &currentSes,
	}
}

func (m *Model) Init() tea.Cmd {
	m.updateHeader()
	m.updateFooter()
	m.chatLog.AddSystem("Welcome to AnyClaw! Type /help for available commands.")
	return tea.Batch(
		m.editor.Focus(),
		m.spinner.Tick,
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chatLog.SetSize(msg.Width, msg.Height-4)
		m.editor.SetWidth(msg.Width - 2)

	case tea.KeyMsg:
		if m.showOverlay {
			cmd := m.handleOverlayKey(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			cmd := m.handleKeyMsg(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case spinner.TickMsg:
		if m.loading {
			m.spinner, _ = m.spinner.Update(msg)
		}

	case LLMResponseMsg:
		m.chatLog.AddMessage("assistant", msg.Content)
		m.lastResponse = msg.Content
		m.loading = false
		m.updateFooter()

	case LLMStreamMsg:
		if len(m.chatLog.messages) > 0 && m.chatLog.messages[len(m.chatLog.messages)-1].Role == "assistant" {
			m.chatLog.messages[len(m.chatLog.messages)-1].Content += msg.Content
		} else {
			m.chatLog.AddMessage("assistant", msg.Content)
		}
		m.chatLog.updateViewportContent()

	case ThinkingMsg:
		m.chatLog.AddThinking(msg.Content)

	case ToolCallMsg:
		m.chatLog.AddToolCall(msg.ToolName, msg.Content)

	case LoadingDoneMsg:
		m.loading = false
		if !m.startTime.IsZero() {
			elapsed := time.Since(m.startTime)
			m.setStatusMessage(fmt.Sprintf("completed in %s", elapsed.Round(time.Second)))
		}

	case LoadingStartMsg:
		m.loading = true
		m.startTime = time.Now()
		cmds = append(cmds, m.spinner.Tick)
	}

	cmd := m.editor.Update(msg)
	_ = cmd
	m.chatLog.Update(msg)

	return m, tea.Batch(cmds...)
}

func (m *Model) handleOverlayKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyUp:
		if m.overlay.selectedIdx > 0 {
			m.overlay.selectedIdx--
		}
	case tea.KeyDown:
		if m.overlay.selectedIdx < len(m.overlay.items)-1 {
			m.overlay.selectedIdx++
		}
	case tea.KeyEnter:
		if m.overlay.selectedIdx < len(m.overlay.items) {
			m.overlay.onSelect(m.overlay.items[m.overlay.selectedIdx])
		}
		m.closeOverlay()
	case tea.KeyEscape:
		m.closeOverlay()
	case tea.KeyCtrlC:
		m.closeOverlay()
	}
	return nil
}

func (m *Model) closeOverlay() {
	m.showOverlay = false
	m.overlay = Overlay{}
}

func (m *Model) openOverlay(o Overlay) {
	m.showOverlay = true
	m.overlay = o
}

func (m *Model) handleKeyMsg(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD:
		if m.onQuit != nil {
			m.onQuit()
		}
		return tea.Quit

	case tea.KeyEscape:
		m.chatLog.AddSystem("Press Ctrl+C to exit")

	case tea.KeyEnter:
		value := m.editor.GetValue()
		if value == "" {
			return nil
		}

		m.editor.AddToHistory(value)

		if value[0] == '/' {
			return m.handleCommand(value)
		}

		m.chatLog.AddMessage("user", value)
		m.editor.Clear()
		m.loading = true
		m.startTime = time.Now()

		return m.sendToAgentWithStreaming(value)

	case tea.KeyTab:
		if m.editor.IsShowingSuggestions() {
			m.completeCommand()
		}
		return nil

	case tea.KeyCtrlL:
		m.openModelSelector()
		return nil

	case tea.KeyCtrlP:
		m.openProviderSelector()
		return nil

	case tea.KeyCtrlG:
		m.openAgentSelector()
		return nil

	case tea.KeyCtrlS:
		m.openSettings()
		return nil

	case tea.KeyCtrlT:
		m.toggleThinking()
		return nil

	case tea.KeyCtrlV:
		m.toggleVerbose()
		return nil

	case tea.KeyCtrlO:
		m.chatLog.showTools = !m.chatLog.showTools
		m.chatLog.updateViewportContent()
		m.setStatusMessage(fmt.Sprintf("tools %s", map[bool]string{true: "expanded", false: "collapsed"}[m.chatLog.showTools]))
		return nil

	case tea.KeyCtrlR:
		m.reloadHistory()
		return nil

	case tea.KeyCtrlH:
		m.toggleTheme()
		return nil

	case tea.KeyRight:
		if m.currentHelpPage < len(m.helpPages)-1 {
			m.currentHelpPage++
			m.showHelpPage()
		}
		return nil

	case tea.KeyLeft:
		if m.currentHelpPage > 0 {
			m.currentHelpPage--
			m.showHelpPage()
		}
		return nil
	}

	return nil
}

func (m *Model) handleCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	cmd := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	switch cmd {
	case "/help", "/?":
		m.showHelp()
	case "/exit", "/quit", "/q":
		if m.onQuit != nil {
			m.onQuit()
		}
		return tea.Quit
	case "/clear", "/reset":
		m.chatLog.Clear()
		m.agent.ClearHistory()
		m.setStatusMessage("chat history cleared")
	case "/memory":
		mem, _ := m.agent.ShowMemory()
		m.chatLog.AddSystem(fmt.Sprintf("Memory:\n%s", mem))
	case "/skills":
		m.showSkillsList()
	case "/tools":
		m.showToolsList()
	case "/provider":
		m.showProviderInfo()
	case "/providers":
		m.showProviders()
	case "/agents":
		m.showAgents()
	case "/audit":
		m.chatLog.AddSystem("Audit log: not implemented yet")
	case "/think":
		m.handleThink(args)
	case "/verbose":
		m.handleVerbose(args)
	case "/status":
		m.showStatus()
	case "/abort":
		m.setStatusMessage("abort requested")
	case "/set":
		m.handleSet(args)
	default:
		if strings.HasPrefix(cmd, "/") {
			m.chatLog.AddSystem(fmt.Sprintf("Unknown command: %s. Type /help for available commands.", cmd))
		}
	}
	return nil
}

func (m *Model) handleSet(args string) {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		m.chatLog.AddSystem("Usage: /set <provider|model|temp> <value>")
		return
	}

	key := parts[0]
	value := parts[1]

	switch key {
	case "provider":
		if err := m.llm.SwitchProvider(value); err != nil {
			m.chatLog.AddSystem(fmt.Sprintf("Failed to switch provider: %v", err))
		} else {
			m.chatLog.AddSystem(fmt.Sprintf("Provider switched to: %s", value))
			m.updateFooter()
		}
	case "model":
		if err := m.llm.SwitchModel(value); err != nil {
			m.chatLog.AddSystem(fmt.Sprintf("Failed to switch model: %v", err))
		} else {
			m.chatLog.AddSystem(fmt.Sprintf("Model switched to: %s", value))
			m.updateFooter()
		}
	case "temp", "temperature":
		var temp float64
		fmt.Sscanf(value, "%f", &temp)
		m.llm.SetTemperature(temp)
		m.chatLog.AddSystem(fmt.Sprintf("Temperature set to: %.1f", temp))
	default:
		m.chatLog.AddSystem(fmt.Sprintf("Unknown setting: %s", key))
	}
}

func (m *Model) handleThink(level string) {
	levels := map[string]string{"off": "off", "minimal": "minimal", "low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh"}
	if level == "" {
		m.chatLog.AddSystem(fmt.Sprintf("Thinking: %s (use /think <level> to change)", m.thinkingLevel))
		return
	}
	if _, ok := levels[level]; ok {
		m.thinkingLevel = level
		m.chatLog.AddSystem(fmt.Sprintf("Thinking set to: %s", level))
	} else {
		m.chatLog.AddSystem(fmt.Sprintf("Invalid level. Use: off, minimal, low, medium, high, xhigh"))
	}
}

func (m *Model) handleVerbose(level string) {
	levels := map[string]string{"off": "off", "on": "on"}
	if level == "" {
		m.chatLog.AddSystem(fmt.Sprintf("Verbose: %s (use /verbose <on|off> to change)", m.verboseLevel))
		return
	}
	if _, ok := levels[level]; ok {
		m.verboseLevel = level
		m.chatLog.AddSystem(fmt.Sprintf("Verbose set to: %s", level))
	} else {
		m.chatLog.AddSystem(fmt.Sprintf("Invalid level. Use: on, off"))
	}
}

func (m *Model) sendToAgent(message string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		resp, err := m.agent.Run(ctx, message)
		if err != nil {
			m.chatLog.AddSystem(fmt.Sprintf("Error: %v", err))
			return LoadingDoneMsg{}
		}

		m.chatLog.AddMessage("assistant", resp)
		m.lastResponse = resp
		m.saveSession()
		return LoadingDoneMsg{}
	}
}

func (m *Model) sendToAgentWithStreaming(message string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		m.chatLog.AddMessage("assistant", "")
		lastIdx := len(m.chatLog.messages) - 1

		var mu sync.Mutex
		var fullResponse strings.Builder

		err := m.agent.RunStream(ctx, message, func(chunk string) {
			mu.Lock()
			fullResponse.WriteString(chunk)
			content := fullResponse.String()
			mu.Unlock()

			m.chatLog.UpdateLastMessage(content)
		})

		if err != nil {
			m.chatLog.messages[lastIdx].Content = fmt.Sprintf("Error: %v", err)
			m.chatLog.updateViewportContent()
			m.statusMessage = "error"
			return LoadingDoneMsg{}
		}

		mu.Lock()
		finalContent := fullResponse.String()
		mu.Unlock()

		if finalContent == "" {
			finalContent = "(empty response)"
		}

		m.chatLog.messages[lastIdx].Content = finalContent
		m.chatLog.updateViewportContent()
		m.lastResponse = finalContent
		m.saveSession()
		m.statusMessage = "done"
		return LoadingDoneMsg{}
	}
}

func (m *Model) saveSession() {
	if m.session == nil || m.currentSes == nil {
		return
	}

	m.currentSes.Messages = m.chatLog.messages
	if m.llm != nil {
		m.currentSes.Provider = m.llm.GetProvider()
		m.currentSes.Model = m.llm.GetModel()
	}

	_ = m.session.SaveSession(*m.currentSes)
}

func (m *Model) showHelp() {
	pages := []string{
		`
Interactive Commands:
  /exit, /quit, /q   - Exit program
  /clear, /reset     - Clear chat history
  /memory            - Show memory
  /skills            - List skills
  /tools             - List tools
  /provider          - Show current provider/model
  /providers         - List available providers
  /agents            - List agents
  /audit             - Show audit log
  /think             - Set thinking level
  /verbose           - Set verbose mode
  /status            - Show session status
  /abort             - Abort current request
  /set               - Set configuration
  /agent             - Switch agent
  /help, /?          - Show help
  /sessions          - List saved sessions
  /session load <id> - Load session
  /session save <id> - Save session
`,
		`
Keyboard Shortcuts:
  Ctrl+P - Provider selector
  Ctrl+L - Model selector
  Ctrl+G - Agent selector
  Ctrl+S - Settings
  Ctrl+T - Toggle thinking
  Ctrl+V - Toggle verbose
  Ctrl+O - Toggle tools expanded
  Ctrl+R - Reload history
  ↑/↓    - History navigation
  Tab    - Command completion
  Esc    - Cancel
  Ctrl+C - Exit

Navigation:
  Use ↑/↓ to scroll through chat history
  Use PageUp/PageDown for quick scroll
  Ctrl+L to clear screen
`,
		`
Tips:
  • Start with / to use commands
  • Type normally for AI chat
  • Use Tab to auto-complete commands
  • Press Ctrl+O to see tool calls
  • Sessions auto-save on each message
  
Examples:
  /set provider anthropic
  /set model claude-sonnet-4-7
  /think high
  /verbose on
`,
	}

	m.helpPages = pages
	m.currentHelpPage = 0
	m.showHelpPage()
}

func (m *Model) showHelpPage() {
	if len(m.helpPages) == 0 {
		return
	}
	m.chatLog.AddSystem(m.helpPages[m.currentHelpPage] + fmt.Sprintf("\n[Page %d/%d - Press → for next, ← for previous]", m.currentHelpPage+1, len(m.helpPages)))
}

func (m *Model) showSkillsList() {
	skills := m.agent.ListSkills()
	msg := "Available Skills:\n"
	for _, s := range skills {
		msg += fmt.Sprintf("  • %s: %s\n", s.Name, s.Description)
	}
	m.chatLog.AddSystem(msg)
	m.setStatusMessage(fmt.Sprintf("%d skills", len(skills)))
}

func (m *Model) showToolsList() {
	tools := m.agent.ListTools()
	msg := "Available Tools:\n"
	for _, t := range tools {
		msg += fmt.Sprintf("  • %s: %s\n", t.Name, t.Description)
	}
	m.chatLog.AddSystem(msg)
	m.setStatusMessage(fmt.Sprintf("%d tools", len(tools)))
}

func (m *Model) showProviderInfo() {
	provider := m.llm.GetProvider()
	model := m.llm.GetModel()
	temp := m.llm.GetTemperature()
	m.chatLog.AddSystem(fmt.Sprintf("Provider: %s\nModel: %s\nTemperature: %.1f\nThinking: %s\nVerbose: %s",
		provider, model, temp, m.thinkingLevel, m.verboseLevel))
}

func (m *Model) showProviders() {
	providers := "Available Providers:\n" +
		"  • openai - OpenAI (GPT-4, GPT-3.5)\n" +
		"  • anthropic - Anthropic (Claude)\n" +
		"  • qwen - Alibaba Qwen\n" +
		"  • ollama - Ollama (本地模型)\n" +
		"  • compatible - OpenAI 兼容 API\n\n" +
		"Use /set provider <name> to switch"
	m.chatLog.AddSystem(providers)
}

func (m *Model) showAgents() {
	msg := "Agent Profiles:\n" +
		"  • default - Default agent\n\n" +
		"Use /agent use <name> to switch"
	m.chatLog.AddSystem(msg)
}

func (m *Model) showStatus() {
	provider := m.llm.GetProvider()
	model := m.llm.GetModel()
	temp := m.llm.GetTemperature()
	historyLen := len(m.agent.GetHistory())

	var elapsed string
	if !m.startTime.IsZero() && m.loading {
		elapsed = time.Since(m.startTime).Round(time.Second).String()
	}

	status := fmt.Sprintf("Provider: %s | Model: %s | Temp: %.1f | Thinking: %s | Verbose: %s | History: %d",
		provider, model, temp, m.thinkingLevel, m.verboseLevel, historyLen)
	if elapsed != "" {
		status += " | " + elapsed
	}
	m.chatLog.AddSystem(status)
	m.setStatusMessage("status")
}

func (m *Model) openModelSelector() {
	models := getModelsForProvider(m.llm.GetProvider())
	items := make([]SelectItem, len(models))
	for i, m := range models {
		items[i] = SelectItem{Label: m, Value: m, Desc: ""}
	}

	m.openOverlay(Overlay{
		overlayType: OverlayModel,
		items:       items,
		selectedIdx: 0,
		title:       "Select Model",
		onSelect: func(item SelectItem) {
			m.llm.SwitchModel(item.Value)
			m.chatLog.AddSystem(fmt.Sprintf("Model set to: %s", item.Value))
			m.updateFooter()
		},
		onCancel: func() {},
	})
}

func (m *Model) openProviderSelector() {
	providers := []SelectItem{
		{Label: "openai", Value: "openai", Desc: "OpenAI (GPT-4, GPT-3.5)"},
		{Label: "anthropic", Value: "anthropic", Desc: "Anthropic (Claude)"},
		{Label: "qwen", Value: "qwen", Desc: "Alibaba Qwen"},
		{Label: "ollama", Value: "ollama", Desc: "Ollama (本地模型)"},
		{Label: "compatible", Value: "compatible", Desc: "OpenAI 兼容 API"},
	}

	m.openOverlay(Overlay{
		overlayType: OverlayProvider,
		items:       providers,
		selectedIdx: 0,
		title:       "Select Provider",
		onSelect: func(item SelectItem) {
			m.llm.SwitchProvider(item.Value)
			m.chatLog.AddSystem(fmt.Sprintf("Provider set to: %s", item.Value))
			m.updateFooter()
		},
		onCancel: func() {},
	})
}

func (m *Model) openAgentSelector() {
	agents := []SelectItem{
		{Label: "default", Value: "default", Desc: "Default agent"},
	}

	m.openOverlay(Overlay{
		overlayType: OverlayAgent,
		items:       agents,
		selectedIdx: 0,
		title:       "Select Agent",
		onSelect: func(item SelectItem) {
			m.agentName = item.Value
			m.chatLog.AddSystem(fmt.Sprintf("Agent set to: %s", item.Value))
			m.updateHeader()
		},
		onCancel: func() {},
	})
}

func (m *Model) openSettings() {
	settings := []SelectItem{
		{Label: fmt.Sprintf("Provider: %s", m.llm.GetProvider()), Value: "provider", Desc: "Change provider"},
		{Label: fmt.Sprintf("Model: %s", m.llm.GetModel()), Value: "model", Desc: "Change model"},
		{Label: fmt.Sprintf("Temperature: %.1f", m.llm.GetTemperature()), Value: "temp", Desc: "Change temperature"},
		{Label: fmt.Sprintf("Thinking: %s", m.thinkingLevel), Value: "think", Desc: "Thinking level"},
		{Label: fmt.Sprintf("Verbose: %s", m.verboseLevel), Value: "verbose", Desc: "Verbose mode"},
	}

	m.openOverlay(Overlay{
		overlayType: OverlaySettings,
		items:       settings,
		selectedIdx: 0,
		title:       "Settings",
		onSelect: func(item SelectItem) {
			m.closeOverlay()
			switch item.Value {
			case "provider":
				m.openProviderSelector()
			case "model":
				m.openModelSelector()
			case "temp":
				m.chatLog.AddSystem("Use /set temp <0.0-2.0> to change temperature")
			case "think":
				m.chatLog.AddSystem("Use /think <off|minimal|low|medium|high|xhigh> to change")
			case "verbose":
				m.chatLog.AddSystem("Use /verbose <on|off> to change")
			}
		},
		onCancel: func() {},
	})
}

func (m *Model) toggleThinking() {
	levels := []string{"off", "minimal", "low", "medium", "high", "xhigh"}
	currentIdx := 0
	for i, l := range levels {
		if l == m.thinkingLevel {
			currentIdx = i
			break
		}
	}
	nextIdx := (currentIdx + 1) % len(levels)
	m.thinkingLevel = levels[nextIdx]
	m.chatLog.SetShowThinking(m.thinkingLevel != "off")
	m.setStatusMessage(fmt.Sprintf("thinking %s", m.thinkingLevel))
}

func (m *Model) toggleTheme() {
	ToggleTheme()
	RefreshStyles()
	m.chatLog.updateViewportContent()
	m.updateHeader()
	m.updateFooter()
	m.setStatusMessage(fmt.Sprintf("theme %s", map[ThemeType]string{ThemeDark: "dark", ThemeLight: "light"}[GetCurrentTheme()]))
}

func (m *Model) toggleVerbose() {
	if m.verboseLevel == "on" {
		m.verboseLevel = "off"
	} else {
		m.verboseLevel = "on"
	}
	m.setStatusMessage(fmt.Sprintf("verbose %s", m.verboseLevel))
}

func (m *Model) reloadHistory() {
	history := m.agent.GetHistory()
	m.chatLog.Clear()
	for _, msg := range history {
		m.chatLog.AddMessage(msg.Role, msg.Content)
	}
	m.setStatusMessage(fmt.Sprintf("loaded %d messages", len(history)))
}

func getModelsForProvider(provider string) []string {
	models := map[string][]string{
		"openai":     {"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo"},
		"anthropic":  {"claude-opus-4-5", "claude-sonnet-4-7", "claude-haiku-3-5"},
		"qwen":       {"qwen-plus", "qwen-turbo", "qwen-max", "qwen2.5-72b-instruct", "qwq-32b-preview"},
		"ollama":     {"llama3.2", "llama3.1", "codellama", "mistral"},
		"compatible": {"gpt-4o"},
	}
	if m, ok := models[provider]; ok {
		return m
	}
	return []string{"gpt-4o"}
}

func (m *Model) isShowThinking() bool {
	return m.chatLog.showThink
}

func (m *Model) isToolsExpanded() bool {
	return m.chatLog.showTools
}

func (m *Model) completeCommand() {
	m.editor.CompleteCommand()
}

func (m *Model) setStatusMessage(msg string) {
	m.statusMessage = msg
	m.updateFooter()
}

func (m *Model) updateHeader() {
	connectionStatus := "connected"
	if m.loading {
		connectionStatus = "busy"
	}
	m.header = HeaderStyle.Render(fmt.Sprintf(" AnyClaw - agent %s | %s | %s ", m.agentName, connectionStatus, m.llm.GetModel()))
}

func (m *Model) updateFooter() {
	provider := ""
	model := ""
	if m.llm != nil {
		provider = m.llm.GetProvider()
		model = m.llm.GetModel()
	}

	var footerParts []string
	footerParts = append(footerParts, fmt.Sprintf("agent %s", m.agentName))
	footerParts = append(footerParts, fmt.Sprintf("session main"))
	footerParts = append(footerParts, fmt.Sprintf("%s/%s", provider, model))

	if m.thinkingLevel != "off" {
		footerParts = append(footerParts, fmt.Sprintf("think %s", m.thinkingLevel))
	}
	if m.verboseLevel != "off" {
		footerParts = append(footerParts, fmt.Sprintf("verbose %s", m.verboseLevel))
	}
	if m.statusMessage != "" {
		footerParts = append(footerParts, m.statusMessage)
	}

	footerText := strings.Join(footerParts, " | ")
	m.footer = FooterStyle.Render(footerText)
}

func (m *Model) getHelpBar() string {
	return HelpBarStyle.Render("Tab:补全 | Ctrl+H:主题 | Ctrl+O:工具 | Ctrl+T:思考 | Ctrl+V:详细 | ↑↓:历史")
}

func (m Model) View() string {
	var content string

	content += m.header + "\n"

	if m.loading {
		elapsed := ""
		if !m.startTime.IsZero() {
			elapsed = " | " + time.Since(m.startTime).Round(time.Second).String()
		}
		statusLine := fmt.Sprintf("%s waiting%s", m.spinner.View(), elapsed)
		content += StatusStyle.Render(" "+statusLine) + "\n"
	} else if m.statusMessage != "" {
		content += StatusStyle.Render(" "+m.statusMessage) + "\n"
	}

	content += m.chatLog.View() + "\n"
	content += m.footer + "\n"
	content += m.editor.View()

	if m.showOverlay {
		content = m.renderOverlay(content)
	}

	return content
}

func (m *Model) renderOverlay(baseView string) string {
	lines := strings.Split(baseView, "\n")

	overlayWidth := 50
	overlayHeight := len(m.overlay.items) + 4
	if overlayHeight > m.height-4 {
		overlayHeight = m.height - 4
	}

	overlayY := (m.height - overlayHeight) / 2
	overlayX := (m.width - overlayWidth) / 2

	var overlayLines []string
	overlayLines = append(overlayLines, BorderStyle.Render(
		fmt.Sprintf("%s%s%s", strings.Repeat(" ", overlayX), m.overlay.title, strings.Repeat(" ", overlayWidth-len(m.overlay.title)-2))))

	for i, item := range m.overlay.items {
		prefix := "  "
		if i == m.overlay.selectedIdx {
			prefix = "► "
		}
		line := fmt.Sprintf("%s%s%s", prefix, item.Label, strings.Repeat(" ", overlayWidth-len(prefix)-len(item.Label)-2))
		overlayLines = append(overlayLines, BorderStyle.Render(line))
	}

	overlayLines = append(overlayLines, BorderStyle.Render(
		fmt.Sprintf("%s[↑↓ select, Enter confirm, Esc cancel]%s", strings.Repeat(" ", overlayX+2), strings.Repeat(" ", overlayWidth-30))))

	for i, ol := range overlayLines {
		if overlayY+i < len(lines) {
			lines[overlayY+i] = ol
		}
	}

	return strings.Join(lines, "\n")
}

type LLMResponseMsg struct {
	Content string
}

type LLMStreamMsg struct {
	Content string
}

type ThinkingMsg struct {
	Content string
}

type ToolCallMsg struct {
	ToolName string
	Content  string
}

type LoadingStartMsg struct{}

type LoadingDoneMsg struct{}
