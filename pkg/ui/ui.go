// Package ui provides terminal UI styling utilities using lipgloss.
package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/consoleio"
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

	bannerCardStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#155E75")).Padding(0, 1).Width(96)
	bannerTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E0F2FE"))
	bannerLeadStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
	bannerMetaStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1"))
	bannerHintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	bannerVersionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(lipgloss.Color("#FDE68A")).Padding(0, 1)
	panelStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#334155")).Padding(0, 1)
	panelTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(lipgloss.Color("#7DD3FC")).Padding(0, 1)
	sectionTitle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67E8F9"))
	keyStyle           = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#94A3B8")).Width(10)
	valueStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
	promptLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(lipgloss.Color("#67E8F9"))
	promptArrowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	chatRoleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FAFC")).Background(lipgloss.Color("#0F766E")).Padding(0, 1)
	chatBodyStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#155E75")).Padding(0, 1).MarginLeft(2)
)

func Banner(version string) {
	title := bannerTitleStyle.Render("AnyClaw")
	if version != "" {
		title = lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", bannerVersionStyle.Render("v"+version))
	}

	content := []string{
		lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", bannerLeadStyle.Render("gateway-first AI agent")),
		bannerMetaStyle.Render("chat, tools, files, automation / chinese assistant / file-first workspace"),
		bannerHintStyle.Render("/help commands / /markdown on|off / /clear history / /quit exit"),
	}

	fmt.Printf("\n%s\n\n", bannerCardStyle.Render(strings.Join(content, "\n")))
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
	reader := consoleio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func PromptWithDefault(label, defaultVal string) string {
	fmt.Printf("%s (%s) > ", label, defaultVal)
	reader := consoleio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	val := strings.TrimSpace(line)
	if val == "" {
		return defaultVal
	}
	return val
}

func Confirm(label string) bool {
	fmt.Printf("%s (y/N) > ", label)
	reader := consoleio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	val := strings.TrimSpace(strings.ToLower(line))
	return val == "y" || val == "yes"
}

func KeyValue(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		keyStyle.Render(strings.ToLower(strings.TrimSpace(label))),
		valueStyle.Render(strings.TrimSpace(value)),
	)
}

func InteractivePanel(title string, lines []string, tips []string) string {
	content := []string{panelTitleStyle.Render(title)}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		content = append(content, line)
	}
	if len(tips) > 0 {
		content = append(content, "", bannerHintStyle.Render(strings.Join(tips, "  /  ")))
	}
	return panelStyle.Render(strings.Join(content, "\n"))
}

func SectionTitle(text string) string {
	return sectionTitle.Render(text)
}

func PromptPrefix(label string) string {
	return promptLabelStyle.Render(strings.ToLower(strings.TrimSpace(label))) + " " + promptArrowStyle.Render(">")
}

func ChatHeader(label string) string {
	if strings.TrimSpace(label) == "" {
		label = "assistant"
	}
	return chatRoleStyle.Render(label)
}

func ChatBody(content string) string {
	return chatBodyStyle.Render(content)
}
