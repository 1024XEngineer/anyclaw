package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const DefaultBootstrapMaxChars = 20000

type BootstrapOptions struct {
	AgentName         string
	AgentDescription  string
	BootstrapMaxChars int
	UserProfile       string
	WorkspaceFocus    string
	AssistantStyle    string
	Constraints       string
}

type BootstrapFile struct {
	Name      string
	Content   string
	Missing   bool
	Truncated bool
}

var bootstrapFileOrder = []string{
	"AGENTS.md",
	"SOUL.md",
	"TOOLS.md",
	"IDENTITY.md",
	"USER.md",
	"HEARTBEAT.md",
	"BOOTSTRAP.md",
	"MEMORY.md",
}

func EnsureBootstrap(dir string, opts BootstrapOptions) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "memory"), 0o755); err != nil {
		return err
	}

	existingBootstrap := false
	for _, name := range bootstrapFileOrder {
		if bootstrapFileExists(dir, name) {
			existingBootstrap = true
			break
		}
	}

	templates := defaultBootstrapTemplates(opts)
	for _, name := range bootstrapFileOrder {
		if name == "BOOTSTRAP.md" && existingBootstrap {
			continue
		}
		path := filepath.Join(dir, name)
		if bootstrapFileExists(dir, name) {
			continue
		}
		content, ok := templates[name]
		if !ok {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}

	if shouldAutoCompleteBootstrap(opts) && fileExists(filepath.Join(dir, "BOOTSTRAP.md")) {
		if err := finalizeConfiguredBootstrap(dir, opts); err != nil {
			return err
		}
	}

	return nil
}

func shouldAutoCompleteBootstrap(opts BootstrapOptions) bool {
	seedCount := 0
	for _, value := range []string{
		opts.UserProfile,
		opts.WorkspaceFocus,
		opts.AssistantStyle,
		opts.Constraints,
	} {
		if strings.TrimSpace(value) != "" {
			seedCount++
		}
	}
	return seedCount >= 2
}

func finalizeConfiguredBootstrap(dir string, opts BootstrapOptions) error {
	state, err := loadBootstrapState(dir)
	if err != nil {
		return err
	}
	if state.Answers == nil {
		state.Answers = map[string]string{}
	}

	seedBootstrapAnswer(state.Answers, "user_profile", opts.UserProfile)
	seedBootstrapAnswer(state.Answers, "workspace_focus", opts.WorkspaceFocus)
	seedBootstrapAnswer(state.Answers, "assistant_style", opts.AssistantStyle)
	seedBootstrapAnswer(state.Answers, "constraints", opts.Constraints)

	if err := writeBootstrapProfiles(dir, state, BootstrapRitualOptions{
		AgentName:        opts.AgentName,
		AgentDescription: opts.AgentDescription,
	}); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, "BOOTSTRAP.md")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return clearBootstrapState(dir)
}

func seedBootstrapAnswer(answers map[string]string, key string, value string) {
	if answers == nil {
		return
	}
	if strings.TrimSpace(answers[key]) != "" {
		return
	}
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		answers[key] = trimmed
	}
}

func LoadBootstrapFiles(dir string, opts BootstrapOptions) ([]BootstrapFile, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, nil
	}

	maxChars := opts.BootstrapMaxChars
	if maxChars <= 0 {
		maxChars = DefaultBootstrapMaxChars
	}

	files := make([]BootstrapFile, 0, len(bootstrapFileOrder))
	for _, name := range bootstrapFileOrder {
		path, actualName := resolveBootstrapFilePath(dir, name)

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				files = append(files, BootstrapFile{
					Name:    name,
					Missing: true,
					Content: fmt.Sprintf("(missing workspace file: %s)", name),
				})
				continue
			}
			return nil, err
		}

		content := strings.TrimSpace(normalizeNewlines(string(data)))
		truncated := false
		if utf8.RuneCountInString(content) > maxChars {
			content = truncateRunes(content, maxChars)
			content = strings.TrimSpace(content) + "\n\n[truncated]"
			truncated = true
		}
		if content == "" {
			content = "(empty)"
		}
		files = append(files, BootstrapFile{
			Name:      actualName,
			Content:   content,
			Truncated: truncated,
		})
	}

	return files, nil
}

func bootstrapFileExists(dir string, name string) bool {
	path, _ := resolveBootstrapFilePath(dir, name)
	return fileExists(path)
}

func resolveBootstrapFilePath(dir string, name string) (string, string) {
	path := filepath.Join(dir, name)
	if name == "MEMORY.md" && !fileExists(path) {
		fallback := filepath.Join(dir, "memory.md")
		if fileExists(fallback) {
			return fallback, "memory.md"
		}
	}
	return path, name
}

func HasInjectedMemoryFile(files []BootstrapFile) bool {
	for _, file := range files {
		if file.Missing {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(file.Name), "MEMORY.md") || strings.EqualFold(strings.TrimSpace(file.Name), "memory.md") {
			return true
		}
	}
	return false
}

func defaultBootstrapTemplates(opts BootstrapOptions) map[string]string {
	name := strings.TrimSpace(opts.AgentName)
	if name == "" {
		name = "AnyClaw"
	}
	description := strings.TrimSpace(opts.AgentDescription)
	if description == "" {
		description = "Execution-oriented local AI assistant."
	}

	return map[string]string{
		"AGENTS.md": strings.TrimSpace(fmt.Sprintf(`# AGENTS

## Primary Agent
- Name: %s
- Goal: Complete the user's task safely, end to end, and verify the real outcome.

## Operating Notes
- If a careful person could safely do the task on this machine, the agent should try to do it instead of only describing it.
- Base each next action on current evidence: file state, command output, browser state, window/app state, UI inspection, OCR, or screenshots.
- Work in loops: inspect -> act -> inspect -> adapt -> verify.
- Prefer higher-level tools and workflows before low-level actions.
- When execution is possible, do the work instead of only explaining it.
- Before finishing, confirm that the requested artifact or state change actually exists.
- Leave concise updates during longer tasks and report what changed, what was verified, and what remains blocked.`, name)),
		"SOUL.md": strings.TrimSpace(fmt.Sprintf(`# SOUL

- Identity: %s
- Description: %s
- Style: Calm, direct, action-oriented, human-like, and collaborative.
- Principle: Finish the task, observe reality instead of guessing, verify the result, and surface blockers clearly.
- Boundary: Protect the machine, protected paths, and private data while still completing safe local work.`, name, description)),
		"TOOLS.md": `# TOOLS

- Prefer app workflows or browser automation before raw desktop clicks.
- Use tools to observe the current world state before and after important actions.
- Prefer files, command output, browser state, UI inspection, OCR, screenshots, and app/window state over assumptions.
- Verify important side effects instead of assuming success.
- If the result is not there yet, continue working or switch strategy instead of narrating a guess.
- Use destructive actions only with explicit approval or clear policy coverage.`,
		"IDENTITY.md": strings.TrimSpace(fmt.Sprintf(`# IDENTITY

- Agent: %s
- Description: %s
- Mission: Complete safe local tasks, not just answer questions.
- Role: Human-like local execution agent for this workspace.
- Default language: Match the user's language.`, name, description)),
		"USER.md": `# USER

- Add durable user preferences here.
- Examples: language, tone, formatting, delivery constraints, tool preferences.`,
		"HEARTBEAT.md": `# HEARTBEAT

- During longer work, send brief progress updates.
- When blocked, explain what was tried, what was observed, and what remains needed.
- Finish with what changed, what was verified, what is still unverified, and any remaining risk.`,
		"BOOTSTRAP.md": `# BOOTSTRAP

This workspace was just initialized.

The first real agent run should complete a short one-time onboarding ritual.
It will ask a few questions, update the workspace identity files, and then remove this file automatically.

Review and personalize these files:
- AGENTS.md
- SOUL.md
- IDENTITY.md
- USER.md
- TOOLS.md
- MEMORY.md
`,
		"MEMORY.md": `# MEMORY

No durable project memory has been captured yet.

Add stable facts, preferences, conventions, verified workflows, and important decisions here.`,
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func normalizeNewlines(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	return strings.ReplaceAll(input, "\r", "\n")
}

func truncateRunes(input string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= limit {
		return input
	}
	return string(runes[:limit])
}
