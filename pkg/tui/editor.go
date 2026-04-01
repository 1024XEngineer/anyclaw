package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type Editor struct {
	input           textinput.Model
	history         []string
	historyIdx      int
	prompt          string
	focused         bool
	suggestions     []string
	showSuggestions bool
	selectedIdx     int
}

var commands = []string{
	"/help",
	"/exit",
	"/quit",
	"/q",
	"/clear",
	"/reset",
	"/memory",
	"/skills",
	"/tools",
	"/provider",
	"/providers",
	"/agents",
	"/audit",
	"/think",
	"/verbose",
	"/status",
	"/abort",
	"/set",
	"/agent",
}

var commandDescriptions = map[string]string{
	"/help":      "Show help",
	"/exit":      "Exit program",
	"/quit":      "Exit program",
	"/q":         "Exit program",
	"/clear":     "Clear chat history",
	"/reset":     "Reset chat",
	"/memory":    "Show memory",
	"/skills":    "List skills",
	"/tools":     "List tools",
	"/provider":  "Show current provider",
	"/providers": "List available providers",
	"/agents":    "List agents",
	"/audit":     "Show audit log",
	"/think":     "Set thinking level",
	"/verbose":   "Set verbose mode",
	"/status":    "Show status",
	"/abort":     "Abort current request",
	"/set":       "Set configuration",
	"/agent":     "Switch agent",
}

func NewEditor() Editor {
	ti := textinput.New()
	ti.Placeholder = "Type a message or / for commands..."
	ti.Prompt = PromptStyle.Render("❯ ")
	ti.CharLimit = 0
	ti.Width = 60

	return Editor{
		input:           ti,
		history:         make([]string, 0),
		prompt:          "❯ ",
		focused:         true,
		suggestions:     []string{},
		showSuggestions: false,
		selectedIdx:     0,
	}
}

func (e *Editor) SetPrompt(prompt string) {
	e.prompt = prompt
	e.input.Prompt = PromptStyle.Render(prompt + " ")
}

func (e *Editor) SetWidth(width int) {
	e.input.Width = width
}

func (e *Editor) Focus() tea.Cmd {
	e.focused = true
	return e.input.Focus()
}

func (e *Editor) Blur() {
	e.focused = false
	e.input.Blur()
}

func (e *Editor) SetValue(value string) {
	e.input.SetValue(value)
}

func (e *Editor) GetValue() string {
	return e.input.Value()
}

func (e *Editor) Clear() {
	e.input.SetValue("")
}

func (e *Editor) AddToHistory(value string) {
	if value == "" {
		return
	}
	if len(e.history) == 0 || e.history[len(e.history)-1] != value {
		e.history = append(e.history, value)
	}
	e.historyIdx = len(e.history)
}

func (e *Editor) historyPrevious() {
	if len(e.history) == 0 {
		return
	}
	if e.historyIdx > 0 {
		e.historyIdx--
	}
	if e.historyIdx < len(e.history) {
		e.input.SetValue(e.history[e.historyIdx])
	}
}

func (e *Editor) historyNext() {
	if len(e.history) == 0 {
		return
	}
	if e.historyIdx < len(e.history) {
		e.historyIdx++
	}
	if e.historyIdx >= len(e.history) {
		e.input.SetValue("")
	} else {
		e.input.SetValue(e.history[e.historyIdx])
	}
}

func (e *Editor) updateSuggestions() {
	value := e.input.Value()

	if strings.HasPrefix(value, "/") {
		inputCmd := strings.ToLower(value)
		var matches []string
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, inputCmd) {
				matches = append(matches, cmd)
			}
		}
		e.suggestions = matches
		e.showSuggestions = len(matches) > 0
		e.selectedIdx = 0
	} else {
		e.suggestions = []string{}
		e.showSuggestions = false
	}
}

func (e *Editor) completeCommand() {
	if len(e.suggestions) > 0 && e.selectedIdx < len(e.suggestions) {
		e.input.SetValue(e.suggestions[e.selectedIdx] + " ")
		e.showSuggestions = false
		e.suggestions = []string{}
	}
}

func (e *Editor) CompleteCommand() {
	e.completeCommand()
}

func (e *Editor) nextSuggestion() {
	if len(e.suggestions) > 0 {
		e.selectedIdx = (e.selectedIdx + 1) % len(e.suggestions)
	}
}

func (e *Editor) prevSuggestion() {
	if len(e.suggestions) > 0 {
		e.selectedIdx--
		if e.selectedIdx < 0 {
			e.selectedIdx = len(e.suggestions) - 1
		}
	}
}

func (e *Editor) GetSuggestions() []string {
	return e.suggestions
}

func (e *Editor) IsShowingSuggestions() bool {
	return e.showSuggestions
}

func (e *Editor) GetSelectedIndex() int {
	return e.selectedIdx
}

func (e *Editor) GetCommandDescription(cmd string) string {
	if desc, ok := commandDescriptions[cmd]; ok {
		return desc
	}
	return ""
}

func (e *Editor) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if e.showSuggestions {
				e.prevSuggestion()
			} else {
				e.historyPrevious()
			}
		case tea.KeyDown:
			if e.showSuggestions {
				e.nextSuggestion()
			} else {
				e.historyNext()
			}
		case tea.KeyTab:
			if e.showSuggestions {
				e.completeCommand()
			}
		}
		e.updateSuggestions()
	}

	_, cmd := e.input.Update(msg)
	e.updateSuggestions()
	return cmd
}

func (e Editor) View() string {
	if e.focused {
		return e.input.View()
	}
	return ""
}

func (e Editor) IsFocused() bool {
	return e.focused
}
