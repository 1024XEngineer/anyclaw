package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/clihub"
	"github.com/anyclaw/anyclaw/pkg/prompt"
)

type IntentPreprocessor struct {
	registry  *clihub.CapabilityRegistry
	execFunc  func([]string, string) (string, error)
	shellName string
}

func NewIntentPreprocessor(root string, execFunc func([]string, string) (string, error)) (*IntentPreprocessor, error) {
	registry, err := clihub.LoadCapabilityRegistry(root)
	if err != nil {
		return nil, err
	}
	return &IntentPreprocessor{
		registry:  registry,
		execFunc:  execFunc,
		shellName: "powershell",
	}, nil
}

type PreprocessResult struct {
	Handled     bool
	Result      string
	Capability  *clihub.Capability
	Confidence  float64
	Description string
}

func (p *IntentPreprocessor) Preprocess(userInput string, args []string) *PreprocessResult {
	query := strings.TrimSpace(userInput)
	if query == "" {
		return nil
	}

	matches := p.registry.FindByIntent(query)
	if len(matches) == 0 {
		return nil
	}

	cap := &matches[0]

	confidence := 1.0
	for _, match := range matches {
		score := 0
		queryLower := strings.ToLower(query)

		if strings.Contains(queryLower, strings.ToLower(match.Harness)) {
			score += 3
		}
		if strings.Contains(queryLower, strings.ToLower(match.Command)) {
			score += 3
		}
		if strings.Contains(queryLower, strings.ToLower(match.Group)) {
			score += 2
		}

		if score > 0 && float64(score)/float64(len(queryLower)) > 0.1 {
			confidence = float64(score) / float64(len(queryLower))
			cap = &match
		}
	}

	if cap == nil {
		return nil
	}

	description := fmt.Sprintf("Auto-executing %s/%s", cap.Harness, cap.Command)

	return &PreprocessResult{
		Handled:     true,
		Capability:  cap,
		Confidence:  confidence,
		Description: description,
	}
}

func (p *IntentPreprocessor) Execute(result *PreprocessResult, additionalArgs []string) (string, error) {
	if result == nil || result.Capability == nil {
		return "", fmt.Errorf("no result to execute")
	}

	cap := result.Capability

	cmdArgs, cwd, err := clihub.ResolveCapabilityPath("", *cap)
	if err != nil {
		return "", err
	}

	fullArgs := append(cmdArgs, cap.Command)
	fullArgs = append(fullArgs, additionalArgs...)
	fullArgs = append([]string{"--json"}, fullArgs...)

	quoted := make([]string, 0, len(fullArgs))
	for _, arg := range fullArgs {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'"'"'`)+"'")
	}
	command := strings.Join(quoted, " ")

	return p.execFunc([]string{command}, cwd)
}

func (p *IntentPreprocessor) GetCapability(name string) *clihub.Capability {
	for _, cap := range p.registry.All() {
		fullName := fmt.Sprintf("%s_%s", cap.Harness, cap.Command)
		if strings.EqualFold(fullName, name) {
			return &cap
		}
	}
	return nil
}

func (p *IntentPreprocessor) ListCapabilities() []clihub.Capability {
	return p.registry.All()
}

func (p *IntentPreprocessor) Count() int {
	return p.registry.Count()
}

type AgentWithIntent struct {
	*Agent
	intent *IntentPreprocessor
}

func NewAgentWithIntent(cfg Config, intent *IntentPreprocessor) *AgentWithIntent {
	return &AgentWithIntent{
		Agent:  New(cfg),
		intent: intent,
	}
}

func (a *AgentWithIntent) RunWithIntent(ctx context.Context, userInput string, autoArgs ...string) (string, error) {
	if a.intent == nil {
		return a.Agent.Run(ctx, userInput)
	}

	result := a.intent.Preprocess(userInput, autoArgs)
	if result == nil || !result.Handled || result.Confidence < 0.15 {
		return a.Agent.Run(ctx, userInput)
	}

	execResult, err := a.intent.Execute(result, autoArgs)
	if err != nil {
		return fmt.Sprintf("[Intent Auto-Failed] %v", err), nil
	}

	a.history = append(a.history, prompt.Message{Role: "user", Content: userInput})
	a.history = append(a.history, prompt.Message{Role: "assistant", Content: execResult})

	return execResult, nil
}

func (a *AgentWithIntent) CanHandleIntent(query string) bool {
	result := a.intent.Preprocess(query, nil)
	return result != nil && result.Handled && result.Confidence >= 0.15
}
