package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// Styles holds all lipgloss styles for the TUI.
type Styles struct {
	// Colors
	Green  lipgloss.Color
	Cyan   lipgloss.Color
	Red    lipgloss.Color
	Yellow lipgloss.Color
	Purple lipgloss.Color
	Gray   lipgloss.Color
	White  lipgloss.Color

	// Text styles
	Bold    lipgloss.Style
	Dim     lipgloss.Style
	Success lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style
	Warning lipgloss.Style

	// Layout
	ChatTabActive   lipgloss.Style
	ChatTabInactive lipgloss.Style
	UserBubble      lipgloss.Style
	AssistantBubble lipgloss.Style
	AgentLabel      lipgloss.Style
	StatusBar       lipgloss.Style
	SessionItem     lipgloss.Style
}

// NewStyles creates the default style set.
func NewStyles() Styles {
	green := lipgloss.Color("#10B981")
	cyan := lipgloss.Color("#06B6D4")
	red := lipgloss.Color("#EF4444")
	yellow := lipgloss.Color("#F59E0B")
	purple := lipgloss.Color("#7C3AED")
	gray := lipgloss.Color("#6B7280")
	white := lipgloss.Color("#F9FAFB")

	return Styles{
		Green:  green,
		Cyan:   cyan,
		Red:    red,
		Yellow: yellow,
		Purple: purple,
		Gray:   gray,
		White:  white,

		Bold:    lipgloss.NewStyle().Bold(true),
		Dim:     lipgloss.NewStyle().Foreground(gray),
		Success: lipgloss.NewStyle().Foreground(green).Bold(true),
		Error:   lipgloss.NewStyle().Foreground(red).Bold(true),
		Info:    lipgloss.NewStyle().Foreground(cyan),
		Warning: lipgloss.NewStyle().Foreground(yellow).Bold(true),

		ChatTabActive: lipgloss.NewStyle().
			Foreground(white).
			Background(purple).
			Bold(true).
			Padding(0, 2).
			MarginRight(1),

		ChatTabInactive: lipgloss.NewStyle().
			Foreground(gray).
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 2).
			MarginRight(1),

		UserBubble: lipgloss.NewStyle().
			Background(lipgloss.Color("#1E40AF")).
			Foreground(white).
			Padding(0, 1).
			MarginLeft(2),

		AssistantBubble: lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2937")).
			Foreground(white).
			Padding(0, 1).
			MarginLeft(2),

		AgentLabel: lipgloss.NewStyle().
			Foreground(purple).
			Bold(true).
			Padding(0, 1),

		StatusBar: lipgloss.NewStyle().
			Foreground(gray).
			Padding(0, 1).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#374151")),

		SessionItem: lipgloss.NewStyle().
			PaddingLeft(1),
	}
}

// KeyMap defines all keyboard shortcuts.
type KeyMap struct {
	Quit             key.Binding
	Help             key.Binding
	SwitchToChat     key.Binding
	SwitchToSessions key.Binding
	SwitchToStatus   key.Binding
	Submit           key.Binding
	NewSession       key.Binding
	Clear            key.Binding
	Select           key.Binding
	Delete           key.Binding
	NextPanel        key.Binding
	PrevPanel        key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "esc"),
			key.WithHelp("ctrl+c/esc", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		SwitchToChat: key.NewBinding(
			key.WithKeys("1", "c"),
			key.WithHelp("1/c", "chat"),
		),
		SwitchToSessions: key.NewBinding(
			key.WithKeys("2", "s"),
			key.WithHelp("2/s", "sessions"),
		),
		SwitchToStatus: key.NewBinding(
			key.WithKeys("3", "t"),
			key.WithHelp("3/t", "status"),
		),
		Submit: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "send"),
		),
		NewSession: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("ctrl+n", "new session"),
		),
		Clear: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear chat"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select session"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d", "backspace"),
			key.WithHelp("d", "delete session"),
		),
		NextPanel: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		PrevPanel: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),
	}
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Quit, k.Help, k.SwitchToChat, k.SwitchToSessions, k.SwitchToStatus,
	}
}

// FullHelp returns all keybindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Quit, k.Help},
		{k.SwitchToChat, k.SwitchToSessions, k.SwitchToStatus},
		{k.Submit, k.NewSession, k.Clear},
		{k.Select, k.Delete},
		{k.NextPanel, k.PrevPanel},
	}
}
