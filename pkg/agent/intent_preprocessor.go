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

const (
	intentCapabilityThreshold  = 0.15
	intentAutoExecuteThreshold = 0.45
)

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

	var (
		cap        *clihub.Capability
		confidence float64
	)
	for i := range matches {
		match := &matches[i]
		score := intentMatchConfidence(query, match)
		if score > confidence {
			confidence = score
			cap = match
		}
	}

	if cap == nil || confidence < intentCapabilityThreshold {
		return nil
	}

	description := fmt.Sprintf("Auto-routing %s to %s/%s", query, cap.Harness, cap.Command)

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
	if p.execFunc == nil {
		return "", fmt.Errorf("intent execution function is not configured")
	}

	cap := result.Capability

	cmdArgs, cwd, err := clihub.ResolveCapabilityPath("", *cap)
	if err != nil {
		return "", err
	}

	fullArgs := append([]string{}, cmdArgs...)
	fullArgs = append(fullArgs, cap.Command, "--json")
	fullArgs = append(fullArgs, additionalArgs...)

	return p.execFunc(fullArgs, cwd)
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
	if result == nil || !result.Handled || result.Confidence < intentAutoExecuteThreshold {
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
	return result != nil && result.Handled && result.Confidence >= intentCapabilityThreshold
}

func intentMatchConfidence(query string, cap *clihub.Capability) float64 {
	if cap == nil {
		return 0
	}

	normalizedQuery := normalizeIntentText(query)
	if normalizedQuery == "" {
		return 0
	}

	score := 0.0
	if containsIntentFragment(normalizedQuery, cap.Harness) {
		score += 3
	}
	if containsIntentFragment(normalizedQuery, cap.Command) {
		score += 3
	}
	if containsIntentFragment(normalizedQuery, cap.Group) {
		score += 2
	}
	if containsIntentFragment(normalizedQuery, cap.Category) {
		score += 1
	}

	keywordHits := 0
	for _, kw := range cap.Keywords {
		if containsIntentFragment(normalizedQuery, kw) {
			keywordHits++
			if keywordHits >= 3 {
				break
			}
		}
	}
	score += float64(keywordHits)

	confidence := score / 10.0
	if confidence > 1 {
		return 1
	}
	return confidence
}

func normalizeIntentText(input string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ", `\`, " ", ".", " ", ":", " ")
	return strings.ToLower(strings.TrimSpace(replacer.Replace(input)))
}

func containsIntentFragment(query string, value string) bool {
	value = normalizeIntentText(value)
	return value != "" && strings.Contains(query, value)
}
