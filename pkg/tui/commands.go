package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Message types

type errMsg struct{ error }
type sessionsLoadedMsg struct{ sessions []Session }
type statusRefreshMsg struct {
	status   *Status
	channels []ChannelStatus
	agents   []AgentInfo
	skills   []SkillInfo
	memory   []MemoryEntry
}
type chatResponseMsg struct {
	sessionID string
	response  string
	agent     string
	err       error
}
type chatStreamMsg struct{ chunk string }

// Commands

func sendMessageCmd(client *GatewayClient, req ChatRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.SendMessage(req)
		if err != nil {
			return chatResponseMsg{err: err}
		}
		return chatResponseMsg{
			sessionID: resp.SessionID,
			response:  resp.Response,
			agent:     resp.Agent,
		}
	}
}

func refreshStatusCmd(client *GatewayClient) tea.Cmd {
	return func() tea.Msg {
		status, _ := client.GetStatus()
		channels, _ := client.ListChannels()
		agents, _ := client.ListAgents()
		skills, _ := client.ListSkills()
		memory, _ := client.GetMemory(10)

		return statusRefreshMsg{
			status:   status,
			channels: channels,
			agents:   agents,
			skills:   skills,
			memory:   memory,
		}
	}
}

func (m *TUI) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.client.ListSessions()
		if err != nil {
			return errMsg{err}
		}
		return sessionsLoadedMsg{sessions: sessions}
	}
}

func (m *TUI) loadStatusData() tea.Cmd {
	return func() tea.Msg {
		status, _ := m.client.GetStatus()
		channels, _ := m.client.ListChannels()
		agents, _ := m.client.ListAgents()
		skills, _ := m.client.ListSkills()
		memory, _ := m.client.GetMemory(10)

		return statusRefreshMsg{
			status:   status,
			channels: channels,
			agents:   agents,
			skills:   skills,
			memory:   memory,
		}
	}
}

// Run launches the TUI application.
func Run(client *GatewayClient) error {
	model := NewTUI(client)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// RunWithSession launches the TUI with a specific session.
func RunWithSession(client *GatewayClient, sessionID string) error {
	model := NewTUI(client)
	model.sessionID = sessionID
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Auto-refresh status periodically
func autoRefreshCmd(client *GatewayClient) tea.Cmd {
	return tea.Tick(15*time.Second, func(_ time.Time) tea.Msg {
		return refreshStatusCmd(client)()
	})
}
