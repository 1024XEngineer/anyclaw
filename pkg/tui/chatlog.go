package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	codeBlockRegex    = regexp.MustCompile("(?s)```(\\w*)\\n?(.*?)```")
	inlineCodeRegex   = regexp.MustCompile("`([^`]+)`")
	boldRegex         = regexp.MustCompile("\\*\\*([^*]+)\\*\\*")
	italicRegex       = regexp.MustCompile("\\*([^*]+)\\*")
	headingRegex      = regexp.MustCompile("(?m)^#{1,6}\\s+(.*)$")
	linkRegex         = regexp.MustCompile("\\[([^\\]]+)\\]\\(([^)]+)\\)")
	listItemRegex     = regexp.MustCompile("(?m)^[-*+]\\s+(.*)$")
	numberedListRegex = regexp.MustCompile("(?m)^\\d+\\.\\s+(.*)$")
)

type Message struct {
	Role            string
	Content         string
	Timestamp       time.Time
	IsTool          bool
	ToolName        string
	ToolArgs        string
	ToolResult      string
	IsThinking      bool
	ThinkingContent string
}

type ChatLog struct {
	messages  []Message
	viewport  viewport.Model
	showTools bool
	showThink bool
	maxWidth  int
}

func NewChatLog() ChatLog {
	vp := viewport.New(0, 0)
	return ChatLog{
		messages:  make([]Message, 0),
		viewport:  vp,
		showTools: false,
		showThink: false,
	}
}

func (c *ChatLog) SetSize(width, height int) {
	c.maxWidth = width
	c.viewport.Width = width
	c.viewport.Height = height - 2
	c.viewport.YPosition = 1
}

func (c *ChatLog) AddMessage(role, content string) {
	c.messages = append(c.messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	c.updateViewportContent()
}

func (c *ChatLog) AddThinking(content string) {
	if len(c.messages) > 0 && c.messages[len(c.messages)-1].Role == "assistant" {
		c.messages[len(c.messages)-1].Content += "\n" + content
	} else {
		c.messages = append(c.messages, Message{
			Role:      "assistant",
			Content:   "🤔 " + content,
			Timestamp: time.Now(),
		})
	}
	c.updateViewportContent()
}

func (c *ChatLog) AddThinkingContent(content string) {
	if len(c.messages) > 0 {
		lastMsg := &c.messages[len(c.messages)-1]
		if lastMsg.Role == "assistant" {
			lastMsg.IsThinking = true
			lastMsg.ThinkingContent = content
			c.updateViewportContent()
		}
	}
}

func (c *ChatLog) StopThinking() {
	if len(c.messages) > 0 {
		lastMsg := &c.messages[len(c.messages)-1]
		if lastMsg.Role == "assistant" && lastMsg.IsThinking {
			lastMsg.IsThinking = false
			lastMsg.Content = lastMsg.ThinkingContent
			c.updateViewportContent()
		}
	}
}

func (c *ChatLog) AddToolCall(name, args string) {
	c.messages = append(c.messages, Message{
		Role:      "tool_call",
		ToolName:  name,
		ToolArgs:  args,
		IsTool:    true,
		Timestamp: time.Now(),
	})
	c.updateViewportContent()
}

func (c *ChatLog) AddToolResult(content string) {
	if len(c.messages) > 0 {
		lastMsg := &c.messages[len(c.messages)-1]
		if lastMsg.IsTool && lastMsg.Role == "tool_call" {
			lastMsg.ToolResult = content
			lastMsg.Role = "tool_result"
			c.updateViewportContent()
			return
		}
	}
	c.messages = append(c.messages, Message{
		Role:      "tool_result",
		Content:   content,
		Timestamp: time.Now(),
	})
	c.updateViewportContent()
}

func (c *ChatLog) UpdateLastMessage(content string) {
	if len(c.messages) > 0 {
		lastMsg := &c.messages[len(c.messages)-1]
		lastMsg.Content = content
		c.updateViewportContent()
	}
}

func (c *ChatLog) AddSystem(content string) {
	c.messages = append(c.messages, Message{
		Role:      "system",
		Content:   content,
		Timestamp: time.Now(),
	})
	c.updateViewportContent()
}

func (c *ChatLog) Clear() {
	c.messages = make([]Message, 0)
	c.viewport.SetContent("")
}

func (c *ChatLog) SetToolsExpanded(expanded bool) {
	c.showTools = expanded
	c.updateViewportContent()
}

func (c *ChatLog) SetShowThinking(show bool) {
	c.showThink = show
	c.updateViewportContent()
}

func (c *ChatLog) updateViewportContent() {
	var content strings.Builder

	for _, msg := range c.messages {
		switch msg.Role {
		case "user":
			content.WriteString(UserMessageStyle.Render("┌─ You"))
			content.WriteString("\n")
			content.WriteString(wrapText(msg.Content, c.maxWidth-4))
			content.WriteString("\n")
		case "assistant":
			if msg.IsThinking {
				content.WriteString(ThinkingStyle.Render("🤔 Thinking..."))
				content.WriteString("\n")
				content.WriteString(wrapText(msg.ThinkingContent, c.maxWidth-4))
				content.WriteString("\n")
			} else if msg.Content != "" {
				content.WriteString(AssistantMessageStyle.Render("┌─ AnyClaw"))
				content.WriteString("\n")
				content.WriteString(renderContentWithCodeBlocks(msg.Content, c.maxWidth-4))
				content.WriteString("\n")
			}
		case "tool_call":
			if c.showTools {
				content.WriteString(ToolCallStyle.Render(fmt.Sprintf("┌─ 🔧 Calling: %s", msg.ToolName)))
				content.WriteString("\n")
				if msg.ToolArgs != "" {
					content.WriteString(wrapText(msg.ToolArgs, c.maxWidth-4))
					content.WriteString("\n")
				}
			}
		case "tool_result":
			if c.showTools {
				content.WriteString(ToolResultStyle.Render("└─ Result:"))
				content.WriteString("\n")
				resultWidth := c.maxWidth - 6
				if resultWidth < 20 {
					resultWidth = 20
				}
				content.WriteString(wrapText(msg.ToolResult, resultWidth))
				content.WriteString("\n")
			}
		case "tool":
			if c.showTools {
				content.WriteString(ToolCallStyle.Render(fmt.Sprintf("┌─ 🔧 %s", msg.ToolName)))
				content.WriteString("\n")
				content.WriteString(wrapText(msg.Content, c.maxWidth-4))
				content.WriteString("\n")
			}
		case "system":
			content.WriteString(SystemMessageStyle.Render("┌─ System"))
			content.WriteString("\n")
			content.WriteString(wrapText(msg.Content, c.maxWidth-4))
			content.WriteString("\n")
		}
	}

	c.viewport.SetContent(content.String())
	c.viewport.GotoBottom()
}

func renderContentWithCodeBlocks(content string, width int) string {
	var result strings.Builder
	lastEnd := 0

	matches := codeBlockRegex.FindAllStringSubmatchIndex(content, -1)
	for _, match := range matches {
		if match[0] > lastEnd {
			result.WriteString(wrapText(content[lastEnd:match[0]], width))
			result.WriteString("\n")
		}

		language := content[match[2]:match[3]]
		code := content[match[4]:match[5]]
		result.WriteString(formatCodeBlock(code, language, width))
		result.WriteString("\n")

		lastEnd = match[1]
	}

	if lastEnd < len(content) {
		result.WriteString(wrapText(content[lastEnd:], width))
	}

	return strings.TrimSuffix(result.String(), "\n")
}

func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 60
	}
	contentWidth := maxWidth - 4

	var result strings.Builder
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		renderedLine := renderMarkdownLine(line, contentWidth)
		result.WriteString(renderedLine)
		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n")
}

func renderMarkdownLine(line string, width int) string {
	line = headingRegex.ReplaceAllStringFunc(line, func(match string) string {
		parts := strings.SplitN(match, " ", 2)
		if len(parts) == 2 {
			level := len(parts[0])
			return HeaderStyle.Render(strings.Repeat("#", level) + " " + parts[1])
		}
		return match
	})

	line = boldRegex.ReplaceAllString(line, BoldStyle.Render("$1"))
	line = italicRegex.ReplaceAllString(line, ItalicStyle.Render("$1"))
	line = inlineCodeRegex.ReplaceAllString(line, CodeStyle.Render("$1"))

	line = linkRegex.ReplaceAllStringFunc(line, func(match string) string {
		parts := linkRegex.FindStringSubmatch(match)
		if len(parts) == 3 {
			return LinkStyle.Render(parts[1])
		}
		return match
	})

	lines := lipgloss.NewStyle().Width(width).Render(line)
	return lines
}

func formatCodeBlock(code, language string, width int) string {
	var result strings.Builder
	result.WriteString(CodeBlockStyle.Render("┌─ "+language) + "\n")

	codeLines := strings.Split(code, "\n")
	for _, line := range codeLines {
		styledLine := CodeBlockContentStyle.Width(width - 4).Render(line)
		result.WriteString(styledLine)
		result.WriteString("\n")
	}

	result.WriteString(CodeBlockStyle.Render("└─"))
	return result.String()
}

func (c ChatLog) View() string {
	return c.viewport.View()
}

func (c *ChatLog) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
			_, cmd := c.viewport.Update(msg)
			return cmd
		}
	case tea.WindowSizeMsg:
		c.SetSize(msg.Width, msg.Height)
	}

	return nil
}
