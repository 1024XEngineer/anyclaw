package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Panel represents the current view panel.
type Panel int

const (
	PanelChat Panel = iota
	PanelSessions
	PanelStatus
)

// TUI is the main model.
type TUI struct {
	client    *GatewayClient
	width     int
	height    int
	panel     Panel
	ready     bool
	quitting  bool
	err       error
	statusMsg string

	// Chat
	sessionID   string
	messages    []ChatMessage
	streaming   bool
	streamBuf   string
	input       textarea.Model
	msgViewport viewport.Model

	// Sessions
	sessions    []Session
	sessionList list.Model
	selSession  int

	// Status
	statusData *Status
	channels   []ChannelStatus
	agents     []AgentInfo
	memory     []MemoryEntry
	skills     []SkillInfo

	// UI components
	spinner spinner.Model
	help    help.Model
	keys    KeyMap

	// Styles
	styles Styles
}

// ChatMessage represents a displayed message.
type ChatMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
	Agent     string
}

// NewTUI creates a new TUI model.
func NewTUI(client *GatewayClient) *TUI {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Enter to send, Ctrl+D to submit)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 4096
	ta.SetWidth(60)
	ta.SetHeight(3)
	ta.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	t := &TUI{
		client:  client,
		panel:   PanelChat,
		input:   ta,
		spinner: s,
		help:    help.New(),
		keys:    DefaultKeyMap(),
		styles:  NewStyles(),
	}

	return t
}

// Init implements tea.Model.
func (m *TUI) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadInitialData(),
	)
}

func (m *TUI) loadInitialData() tea.Cmd {
	return func() tea.Msg {
		status, _ := m.client.GetStatus()
		sessions, _ := m.client.ListSessions()
		agents, _ := m.client.ListAgents()
		return initialDataMsg{
			status:   status,
			sessions: sessions,
			agents:   agents,
		}
	}
}

type initialDataMsg struct {
	status   *Status
	sessions []Session
	agents   []AgentInfo
}

// Update implements tea.Model.
func (m *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.resize()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case initialDataMsg:
		m.statusData = msg.status
		m.sessions = msg.sessions
		m.updateSessionList()
		return m, nil

	case statusRefreshMsg:
		m.statusData = msg.status
		m.channels = msg.channels
		m.agents = msg.agents
		m.skills = msg.skills
		m.memory = msg.memory
		return m, tea.Tick(15*time.Second, func(_ time.Time) tea.Msg {
			return refreshStatusCmd(m.client)
		})

	case chatResponseMsg:
		m.streaming = false
		m.streamBuf = ""
		if msg.err != nil {
			m.messages = append(m.messages, ChatMessage{
				Role:      "error",
				Content:   "Error: " + msg.err.Error(),
				Timestamp: time.Now(),
			})
		} else {
			m.messages = append(m.messages, ChatMessage{
				Role:      "assistant",
				Content:   msg.response,
				Timestamp: time.Now(),
				Agent:     msg.agent,
			})
		}
		m.sessionID = msg.sessionID
		m.input.Focus()
		m.scrollToBottom()
		return m, nil

	case chatStreamMsg:
		m.streamBuf += msg.chunk
		m.scrollToBottom()
		return m, nil

	case sessionsLoadedMsg:
		m.sessions = msg.sessions
		m.updateSessionList()
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil
	}

	var cmd tea.Cmd
	switch m.panel {
	case PanelChat:
		m.input, cmd = m.input.Update(msg)
		m.msgViewport, cmd = m.msgViewport.Update(msg)
	case PanelSessions:
		m.sessionList, cmd = m.sessionList.Update(msg)
	case PanelStatus:
	}

	return m, cmd
}

func (m *TUI) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, m.keys.SwitchToChat):
		m.panel = PanelChat
		m.input.Focus()
		return m, nil

	case key.Matches(msg, m.keys.SwitchToSessions):
		m.panel = PanelSessions
		m.input.Blur()
		return m, m.loadSessions()

	case key.Matches(msg, m.keys.SwitchToStatus):
		m.panel = PanelStatus
		m.input.Blur()
		return m, m.loadStatusData()

	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil

	case key.Matches(msg, m.keys.NextPanel):
		m.panel = (m.panel + 1) % 3
		if m.panel == PanelChat {
			m.input.Focus()
		} else {
			m.input.Blur()
		}
		return m, nil

	case key.Matches(msg, m.keys.PrevPanel):
		m.panel = (m.panel + 2) % 3
		if m.panel == PanelChat {
			m.input.Focus()
		} else {
			m.input.Blur()
		}
		return m, nil

	case key.Matches(msg, m.keys.Clear):
		m.messages = nil
		m.streamBuf = ""
		return m, nil
	}

	if m.panel == PanelChat {
		switch {
		case key.Matches(msg, m.keys.Submit):
			return m.submitMessage()
		case key.Matches(msg, m.keys.NewSession):
			return m, m.createNewSession()
		}
	}

	if m.panel == PanelSessions {
		switch {
		case key.Matches(msg, m.keys.Select):
			return m.selectSession()
		case key.Matches(msg, m.keys.Delete):
			return m, m.deleteCurrentSession()
		}
	}

	var cmd tea.Cmd
	switch m.panel {
	case PanelChat:
		m.input, cmd = m.input.Update(msg)
	case PanelSessions:
		m.sessionList, cmd = m.sessionList.Update(msg)
	}
	return m, cmd
}

func (m *TUI) submitMessage() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || m.streaming {
		return m, nil
	}

	m.messages = append(m.messages, ChatMessage{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	})
	m.input.SetValue("")
	m.streaming = true
	m.streamBuf = ""
	m.scrollToBottom()

	return m, tea.Batch(
		m.spinner.Tick,
		sendMessageCmd(m.client, ChatRequest{
			Message:   text,
			SessionID: m.sessionID,
		}),
	)
}

func (m *TUI) selectSession() (tea.Model, tea.Cmd) {
	if len(m.sessions) == 0 {
		return m, nil
	}
	idx := m.sessionList.Index()
	if idx >= len(m.sessions) {
		return m, nil
	}
	s := m.sessions[idx]
	m.sessionID = s.ID
	m.panel = PanelChat
	m.input.Focus()

	m.messages = append(m.messages, ChatMessage{
		Role:      "system",
		Content:   fmt.Sprintf("Switched to session: %s", s.Title),
		Timestamp: time.Now(),
	})
	return m, nil
}

func (m *TUI) deleteCurrentSession() tea.Cmd {
	if len(m.sessions) == 0 {
		return nil
	}
	idx := m.sessionList.Index()
	if idx >= len(m.sessions) {
		return nil
	}
	s := m.sessions[idx]
	_ = m.client.DeleteSession(s.ID)
	return tea.Batch(
		m.loadSessions(),
		func() tea.Msg {
			if m.sessionID == s.ID {
				m.sessionID = ""
				m.messages = nil
			}
			return nil
		},
	)
}

func (m *TUI) createNewSession() tea.Cmd {
	return func() tea.Msg {
		id, err := m.client.CreateSession("New Session", "")
		if err != nil {
			return errMsg{err}
		}
		m.sessionID = id
		m.messages = nil
		m.messages = append(m.messages, ChatMessage{
			Role:      "system",
			Content:   "New session created",
			Timestamp: time.Now(),
		})
		return sessionsLoadedMsg{sessions: m.sessions}
	}
}

func (m *TUI) resize() {
	if !m.ready {
		return
	}

	// Message viewport
	m.msgViewport.Width = m.width - 4
	m.msgViewport.Height = m.height - 12
	m.msgViewport.SetContent(m.renderMessages())

	// Input
	m.input.SetWidth(m.width - 4)

	// Session list
	if m.sessionList.Styles.TitleBar.GetWidth() > 0 {
		m.sessionList.SetSize(m.width-4, m.height-6)
	}

	// Help
	m.help.Width = m.width - 4
}

func (m *TUI) scrollToBottom() {
	if m.ready {
		m.msgViewport.SetContent(m.renderMessages())
		m.msgViewport.GotoBottom()
	}
}

// View implements tea.Model.
func (m *TUI) View() string {
	if !m.ready {
		return "\n  Initializing AnyClaw TUI..."
	}

	if m.quitting {
		return m.styles.Dim.Render("Goodbye!") + "\n"
	}

	var content string
	switch m.panel {
	case PanelChat:
		content = m.chatView()
	case PanelSessions:
		content = m.sessionsView()
	case PanelStatus:
		content = m.statusView()
	}

	// Tab bar
	tabBar := m.renderTabBar()

	// Status bar
	statusBar := m.renderStatusBar()

	// Help
	helpView := m.help.View(m.keys)

	return lipgloss.JoinVertical(lipgloss.Left,
		tabBar,
		content,
		statusBar,
		helpView,
	)
}

func (m *TUI) chatView() string {
	s := m.styles

	// Messages area
	messagesView := m.msgViewport.View()
	if len(m.messages) == 0 {
		messagesView = s.Dim.Render("\n  No messages yet. Start a conversation!")
	}

	// Streaming indicator
	streamView := ""
	if m.streaming {
		streamView = lipgloss.NewStyle().Foreground(s.Cyan).Render(m.spinner.View()) + " " + lipgloss.NewStyle().Foreground(s.Cyan).Render("Thinking...")
		if m.streamBuf != "" {
			streamView += "\n" + s.Dim.Render(m.streamBuf)
		}
	}

	// Error display
	errView := ""
	if m.err != nil {
		errView = s.Error.Render("  Error: "+m.err.Error()) + "\n"
	}

	inputView := m.input.View()

	return lipgloss.JoinVertical(lipgloss.Left,
		errView,
		messagesView,
		streamView,
		"",
		inputView,
	)
}

func (m *TUI) sessionsView() string {
	s := m.styles
	if len(m.sessions) == 0 {
		return "\n" + s.Dim.Render("  No sessions found.") + "\n"
	}

	// Build custom session list items
	items := make([]list.Item, len(m.sessions))
	for i, sess := range m.sessions {
		title := sess.Title
		if title == "" {
			title = "Untitled"
		}
		desc := fmt.Sprintf("%s | %d msgs | %s",
			sess.Agent,
			sess.MessageCount,
			sess.UpdatedAt.Format("15:04"))
		if sess.LastUserText != "" {
			truncated := sess.LastUserText
			if len(truncated) > 50 {
				truncated = truncated[:47] + "..."
			}
			desc = truncated + " | " + desc
		}
		items[i] = list.Item(sessionItem{
			title:       title,
			description: desc,
			index:       i,
		})
	}

	m.sessionList.SetItems(items)

	return m.sessionList.View()
}

type sessionItem struct {
	title       string
	description string
	index       int
}

func (i sessionItem) Title() string       { return i.title }
func (i sessionItem) Description() string { return i.description }
func (i sessionItem) FilterValue() string { return i.title }

func (m *TUI) updateSessionList() {
	items := make([]list.Item, len(m.sessions))
	for i, sess := range m.sessions {
		title := sess.Title
		if title == "" {
			title = "Untitled"
		}
		desc := fmt.Sprintf("%s | %d msgs | %s",
			sess.Agent,
			sess.MessageCount,
			sess.UpdatedAt.Format("15:04"))
		if sess.LastUserText != "" {
			truncated := sess.LastUserText
			if len(truncated) > 50 {
				truncated = truncated[:47] + "..."
			}
			desc = truncated + " | " + desc
		}
		items[i] = sessionItem{
			title:       title,
			description: desc,
			index:       i,
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	l := list.New(items, delegate, m.width-4, m.height-8)
	l.Title = "Sessions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	m.sessionList = l
}

func (m *TUI) statusView() string {
	s := m.styles

	if m.statusData == nil {
		return "\n" + s.Dim.Render("  Loading status...") + "\n"
	}

	var lines []string

	// Gateway info
	lines = append(lines, s.Bold.Render("  Gateway Status"))
	lines = append(lines, fmt.Sprintf("  Version:   %s", m.statusData.Version))
	lines = append(lines, fmt.Sprintf("  Provider:  %s", m.statusData.Provider))
	lines = append(lines, fmt.Sprintf("  Model:     %s", m.statusData.Model))
	lines = append(lines, fmt.Sprintf("  Address:   %s", m.statusData.Address))
	lines = append(lines, "")

	// Counts
	lines = append(lines, s.Bold.Render("  Counts"))
	lines = append(lines, fmt.Sprintf("  Sessions:  %d", m.statusData.Sessions))
	lines = append(lines, fmt.Sprintf("  Events:    %d", m.statusData.Events))
	lines = append(lines, fmt.Sprintf("  Skills:    %d", m.statusData.Skills))
	lines = append(lines, fmt.Sprintf("  Tools:     %d", m.statusData.Tools))
	lines = append(lines, "")

	// Channels
	if len(m.channels) > 0 {
		lines = append(lines, s.Bold.Render("  Channels"))
		for _, ch := range m.channels {
			status := "⬛"
			if ch.Enabled && ch.Running {
				status = "🟢"
			} else if ch.Enabled {
				status = "🟡"
			} else {
				status = "⬛"
			}
			lines = append(lines, fmt.Sprintf("  %s %-12s %s", status, ch.Name, ch.LastError))
		}
		lines = append(lines, "")
	}

	// Agents
	if len(m.agents) > 0 {
		lines = append(lines, s.Bold.Render("  Agents"))
		for _, a := range m.agents {
			lines = append(lines, fmt.Sprintf("  • %s (%s)", a.Name, a.Domain))
		}
		lines = append(lines, "")
	}

	// Memory
	if len(m.memory) > 0 {
		lines = append(lines, s.Bold.Render(fmt.Sprintf("  Memory (%d entries)", len(m.memory))))
		for _, me := range m.memory {
			truncated := me.Content
			if len(truncated) > 60 {
				truncated = truncated[:57] + "..."
			}
			lines = append(lines, fmt.Sprintf("  [%s] %s", me.Type, truncated))
		}
	}

	return strings.Join(lines, "\n")
}

func (m *TUI) renderTabBar() string {
	s := m.styles
	tabs := []string{"Chat", "Sessions", "Status"}
	var parts []string
	for i, tab := range tabs {
		var style lipgloss.Style
		if Panel(i) == m.panel {
			style = s.ChatTabActive
		} else {
			style = s.ChatTabInactive
		}
		parts = append(parts, style.Render(tab))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m *TUI) renderStatusBar() string {
	s := m.styles
	parts := []string{}

	if m.statusData != nil {
		parts = append(parts, s.Info.Render(fmt.Sprintf(" %s/%s", m.statusData.Provider, m.statusData.Model)))
	}
	if m.sessionID != "" {
		shortID := m.sessionID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		parts = append(parts, s.Dim.Render(" session:"+shortID))
	}
	if m.streaming {
		parts = append(parts, lipgloss.NewStyle().Foreground(s.Cyan).Render(" ⏳ streaming"))
	}

	return s.StatusBar.Render(strings.Join(parts, " │"))
}

func (m *TUI) renderMessages() string {
	s := m.styles
	var lines []string

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			bubble := s.UserBubble.Render(" " + msg.Content + " ")
			lines = append(lines, bubble)
			lines = append(lines, "")
		case "assistant":
			agentLabel := ""
			if msg.Agent != "" {
				agentLabel = s.AgentLabel.Render(" "+msg.Agent+" ") + " "
			}
			bubble := s.AssistantBubble.Render(" " + msg.Content + " ")
			lines = append(lines, agentLabel+bubble)
			lines = append(lines, "")
		case "system":
			lines = append(lines, s.Dim.Render("  ── "+msg.Content+" ──"))
			lines = append(lines, "")
		case "error":
			lines = append(lines, s.Error.Render("  ✗ "+msg.Content))
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}
