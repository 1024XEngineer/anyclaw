// Package ui provides terminal UI styling utilities using lipgloss.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Style wraps lipgloss.Style with Sprint method for compatibility.
type Style struct {
	style lipgloss.Style
}

func (s Style) Sprint(a ...interface{}) string {
	return s.style.Render(fmt.Sprint(a...))
}

func (s Style) Sprintf(format string, a ...interface{}) string {
	return s.style.Render(fmt.Sprintf(format, a...))
}

// Compatibility aliases (used as ui.Bold, ui.Dim, etc.)
var (
	Bold    = Style{style: lipgloss.NewStyle().Bold(true)}
	Dim     = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("242"))}
	Green   = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))}
	Cyan    = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4"))}
	Red     = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))}
	Yellow  = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))}
	Reset   = Style{style: lipgloss.NewStyle()}
	Success = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))}
	Error   = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))}
	Info    = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))}
	Warning = Style{style: lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))}
)

func Banner(version string) {
	fmt.Printf("\n")
	fmt.Printf("  %s\n", Bold.Sprint("AnyClaw"))
	fmt.Printf("  %s\n", Dim.Sprint("File-first AI agent workspace"))
	if version != "" {
		fmt.Printf("  %s\n", Dim.Sprint("v"+version))
	}
	fmt.Printf("\n")
}

type SpinnerModel struct {
	spinner  spinner.Model
	message  string
	quitting bool
}

func NewSpinner(msg string) *SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))
	return &SpinnerModel{spinner: s, message: msg}
}

func (m *SpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *SpinnerModel) View() string {
	if m.quitting {
		return ""
	}
	return m.spinner.View() + " " + m.message
}

func RunSpinner(msg string, fn func() error) error {
	s := NewSpinner(msg)
	p := tea.NewProgram(s, tea.WithOutput(os.Stderr))
	go func() {
		err := fn()
		s.quitting = true
		p.Quit()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s %v\n", Error.Sprint("Error:"), err)
		}
	}()
	_, _ = p.Run()
	return nil
}

func Prompt(label string) string {
	fmt.Printf("%s > ", label)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func PromptWithDefault(label, defaultVal string) string {
	fmt.Printf("%s (%s) > ", label, defaultVal)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	val := strings.TrimSpace(line)
	if val == "" {
		return defaultVal
	}
	return val
}

func Confirm(label string) bool {
	fmt.Printf("%s (y/N) > ", label)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	val := strings.TrimSpace(strings.ToLower(line))
	return val == "y" || val == "yes"
}
