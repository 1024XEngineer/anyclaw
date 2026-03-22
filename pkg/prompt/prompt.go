package prompt

import (
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
	parts = append(parts, b.buildIdentity())
	parts = append(parts, b.buildCapabilities(data))
	parts = append(parts, b.buildMemory(data))
	parts = append(parts, b.buildSkills(data))
	parts = append(parts, b.buildGuidelines())
	parts = append(parts, b.buildInstructions())

	return strings.Join(parts, "\n\n"), nil
}

func (b *SystemPromptBuilder) buildHeader() string {
	return `You are a helpful AI assistant.`
}

func (b *SystemPromptBuilder) buildIdentity() string {
	var parts []string

	if b.name != "" {
		parts = append(parts, fmt.Sprintf("Your name is %s.", b.name))
	}
	if b.description != "" {
		parts = append(parts, b.description)
	}
	parts = append(parts, "You have a configurable personality profile. Follow the tone, style, constraints, and operating traits provided in your identity description.")

	return strings.Join(parts, " ")
}

func (b *SystemPromptBuilder) buildCapabilities(data PromptData) string {
	parts := []string{
		"You have access to the following tools:",
	}

	for _, tool := range data.Tools {
		parts = append(parts, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}

	if len(data.Tools) == 0 {
		parts = append(parts, "(No tools available)")
	}

	return strings.Join(parts, "\n")
}

func (b *SystemPromptBuilder) buildMemory(data PromptData) string {
	if data.Memory == "" {
		return ""
	}

	return fmt.Sprintf(`## Memory
%s`, data.Memory)
}

func (b *SystemPromptBuilder) buildSkills(data PromptData) string {
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

func (b *SystemPromptBuilder) buildGuidelines() string {
	return `## Guidelines
- Be helpful, harmless, and honest
- Think step by step
- When using tools, explain what you're doing
- If a tool fails, explain the error and suggest alternatives
- Store important information in memory for future reference`
}

func (b *SystemPromptBuilder) buildInstructions() string {
	return `## Instructions
- Always respond in the same language as the user
- If tools are available, prefer native structured tool calls; only fall back to textual tool_call JSON if the model cannot emit native tool calls
- After completing a task, summarize what was done`
}

type PromptData struct {
	Name         string
	Description  string
	Memory       string
	Skills       []string
	SkillPrompts []string
	Tools        []ToolInfo
	History      []Message
}

type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Message struct {
	Role    string
	Content string
}

func BuildSystemPrompt(name, description string, data PromptData) (string, error) {
	b := NewSystemPromptBuilder(name, description)
	return b.Build(data)
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
