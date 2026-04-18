// Package ui provides terminal UI styling utilities using lipgloss.
package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/input/cli/consoleio"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	term "github.com/charmbracelet/x/term"
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

	bannerCardStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#155E75")).Padding(0, 1)
	bannerTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E0F2FE"))
	bannerLeadStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
	bannerMetaStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1"))
	bannerHintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	bannerVersionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(lipgloss.Color("#FDE68A")).Padding(0, 1)
	panelStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#334155")).Padding(0, 1)
	panelTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0"))
	sectionTitle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67E8F9"))
	keyStyle           = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#94A3B8")).Width(9)
	valueStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
	promptLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0"))
	promptArrowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
	chatRoleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67E8F9"))
	chatBodyStyle      = lipgloss.NewStyle().BorderLeft(true).BorderForeground(lipgloss.Color("#155E75")).PaddingLeft(1).MarginLeft(1).Foreground(lipgloss.Color("#E2E8F0"))
)

const (
	defaultTerminalWidth = 100
	minRenderableWidth   = 36
	maxBannerWidth       = 112
	maxPanelWidth        = 88
	maxChatWidth         = 100
)

func terminalWidth() int {
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if width, err := strconv.Atoi(raw); err == nil && width > 0 {
			return width
		}
	}

	if width, _, err := term.GetSize(os.Stdout.Fd()); err == nil && width > 0 {
		return width
	}

	return defaultTerminalWidth
}

func boundedWidth(maxWidth int) int {
	width := terminalWidth()
	if maxWidth > 0 && width > maxWidth {
		width = maxWidth
	}
	if width < minRenderableWidth {
		width = minRenderableWidth
	}
	return width
}

func fixedWidthRender(style lipgloss.Style, maxWidth int, content string) string {
	width := boundedWidth(maxWidth) - style.GetHorizontalFrameSize()
	if width < 8 {
		width = 8
	}
	return style.Width(width).Render(content)
}

func wrappedRender(style lipgloss.Style, maxWidth int, content string) string {
	width := boundedWidth(maxWidth) - style.GetHorizontalFrameSize()
	if width < 8 {
		width = 8
	}
	return style.MaxWidth(width).Render(content)
}

func trimRenderedLines(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func formatTips(tips []string) string {
	filtered := make([]string, 0, len(tips))
	for _, tip := range tips {
		tip = strings.TrimSpace(tip)
		if tip == "" {
			continue
		}
		filtered = append(filtered, tip)
	}
	return strings.Join(filtered, " | ")
}

func bannerView(version string) string {
	title := bannerTitleStyle.Render("AnyClaw")
	if version != "" {
		title = lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", bannerVersionStyle.Render("v"+version))
	}

	content := []string{
		lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", bannerLeadStyle.Render("gateway-first AI agent")),
		bannerMetaStyle.Render("chat, tools, files, automation | Chinese assistant | file-first workspace"),
		bannerHintStyle.Render("/help commands | /markdown on|off | /quit exit"),
	}

	return fixedWidthRender(bannerCardStyle, maxBannerWidth, strings.Join(content, "\n"))
}

func Banner(version string) {
	fmt.Printf("\n%s\n\n", bannerView(version))
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
		content = append(content, "", bannerHintStyle.Render(formatTips(tips)))
	}
	return fixedWidthRender(panelStyle, maxPanelWidth, strings.Join(content, "\n"))
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
	return trimRenderedLines(fixedWidthRender(chatBodyStyle, maxChatWidth, content))
}
