package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type Runner struct {
	agent      AgentHandler
	skills     SkillsHandler
	llm        LLMHandler
	onQuitFunc func() error
	workDir    string
}

func NewRunner(
	agent AgentHandler,
	skills SkillsHandler,
	llm LLMHandler,
	onQuitFunc func() error,
	workDir string,
) *Runner {
	return &Runner{
		agent:      agent,
		skills:     skills,
		llm:        llm,
		onQuitFunc: onQuitFunc,
		workDir:    workDir,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	model := NewModel(r.agent, r.skills, r.llm, r.onQuitFunc, r.workDir)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithInputTTY(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

type TUIConfig struct {
	Width  int
	Height int
}

func ShowThinking(spinner spinner.Model) string {
	return fmt.Sprintf("%s Thinking...", spinner.View())
}
