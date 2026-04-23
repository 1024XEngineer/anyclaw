package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const bootstrapStateFilename = ".anyclaw-bootstrap-state.json"

type BootstrapQuestion struct {
	ID     string
	Prompt string
}

type BootstrapState struct {
	Version           int               `json:"version"`
	CurrentIndex      int               `json:"current_index"`
	AwaitingAnswer    bool              `json:"awaiting_answer"`
	Answers           map[string]string `json:"answers,omitempty"`
	PendingUserPrompt string            `json:"pending_user_prompt,omitempty"`
	StartedAt         time.Time         `json:"started_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type BootstrapAdvanceResult struct {
	Active    bool
	Completed bool
	Response  string
}

type BootstrapRitualOptions struct {
	AgentName        string
	AgentDescription string
}

var bootstrapQuestions = []BootstrapQuestion{
	{
		ID:     "user_profile",
		Prompt: "Question 1/4: What should I call you, and what language should I default to when we work together?",
	},
	{
		ID:     "workspace_focus",
		Prompt: "Question 2/4: What kind of work do you mainly want help with in this workspace?",
	},
	{
		ID:     "assistant_style",
		Prompt: "Question 3/4: How should I usually behave: tone, level of detail, speed, and how proactive I should be?",
	},
	{
		ID:     "constraints",
		Prompt: "Question 4/4: Are there any constraints, preferences, or red lines I should always follow?",
	},
}

func AdvanceBootstrapRitual(dir string, userInput string, opts BootstrapRitualOptions) (*BootstrapAdvanceResult, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" || !BootstrapPending(dir) {
		_ = clearBootstrapState(dir)
		return &BootstrapAdvanceResult{}, nil
	}

	state, err := loadBootstrapState(dir)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if state.StartedAt.IsZero() {
		state.StartedAt = now
	}
	state.UpdatedAt = now
	if state.Answers == nil {
		state.Answers = map[string]string{}
	}

	trimmedInput := strings.TrimSpace(userInput)

	if state.CurrentIndex == 0 && trimmedInput == "" && !state.AwaitingAnswer {
		return askBootstrapQuestion(dir, state, opts)
	}

	if !state.AwaitingAnswer {
		if state.CurrentIndex == 0 && trimmedInput != "" && state.PendingUserPrompt == "" {
			state.PendingUserPrompt = trimmedInput
		}
		if err := saveBootstrapState(dir, state); err != nil {
			return nil, err
		}
		return askBootstrapQuestion(dir, state, opts)
	}

	if trimmedInput == "" {
		result, err := currentBootstrapQuestion(state, opts)
		if err != nil {
			return nil, err
		}
		result.Response = "I still need an answer before I can finish workspace bootstrap.\n\n" + result.Response
		return result, nil
	}

	if state.CurrentIndex < 0 || state.CurrentIndex >= len(bootstrapQuestions) {
		state.CurrentIndex = 0
	}
	state.Answers[bootstrapQuestions[state.CurrentIndex].ID] = trimmedInput
	state.CurrentIndex++
	state.AwaitingAnswer = false
	state.UpdatedAt = now

	if state.CurrentIndex >= len(bootstrapQuestions) {
		if err := finalizeBootstrap(dir, state, opts); err != nil {
			return nil, err
		}
		response := "Workspace bootstrap complete. I updated the workspace identity files and removed BOOTSTRAP.md."
		if strings.TrimSpace(state.PendingUserPrompt) != "" {
			response += fmt.Sprintf("\n\nI have not executed your earlier request yet: %s\n\nSend it again and I will continue with the new workspace profile.", state.PendingUserPrompt)
		} else {
			response += "\n\nYou can continue with your real task now."
		}
		return &BootstrapAdvanceResult{
			Active:    true,
			Completed: true,
			Response:  response,
		}, nil
	}

	if err := saveBootstrapState(dir, state); err != nil {
		return nil, err
	}
	result, err := askBootstrapQuestion(dir, state, opts)
	if err != nil {
		return nil, err
	}
	result.Response = "Saved.\n\n" + result.Response
	return result, nil
}

func BootstrapPending(dir string) bool {
	return fileExists(filepath.Join(strings.TrimSpace(dir), "BOOTSTRAP.md"))
}

func askBootstrapQuestion(dir string, state *BootstrapState, opts BootstrapRitualOptions) (*BootstrapAdvanceResult, error) {
	if state == nil {
		state = defaultBootstrapState()
	}
	state.AwaitingAnswer = true
	state.UpdatedAt = time.Now()
	if err := saveBootstrapState(dir, state); err != nil {
		return nil, err
	}
	return currentBootstrapQuestion(state, opts)
}

func currentBootstrapQuestion(state *BootstrapState, opts BootstrapRitualOptions) (*BootstrapAdvanceResult, error) {
	if state.CurrentIndex < 0 || state.CurrentIndex >= len(bootstrapQuestions) {
		return nil, fmt.Errorf("bootstrap question index out of range")
	}
	intro := bootstrapIntro(opts)
	if state.CurrentIndex > 0 {
		intro = ""
	}
	prompt := bootstrapQuestions[state.CurrentIndex].Prompt
	if intro == "" {
		return &BootstrapAdvanceResult{Active: true, Response: prompt}, nil
	}
	return &BootstrapAdvanceResult{Active: true, Response: intro + "\n\n" + prompt}, nil
}

func bootstrapIntro(opts BootstrapRitualOptions) string {
	name := strings.TrimSpace(opts.AgentName)
	if name == "" {
		name = "AnyClaw"
	}
	description := strings.TrimSpace(opts.AgentDescription)
	if description == "" {
		description = "Execution-oriented local AI assistant."
	}
	return fmt.Sprintf("Hello. I am %s, %s.\n\nThis workspace is brand new, so I need a quick setup before we start.", name, description)
}

func finalizeBootstrap(dir string, state *BootstrapState, opts BootstrapRitualOptions) error {
	if err := writeBootstrapProfiles(dir, state, opts); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, "BOOTSTRAP.md")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return clearBootstrapState(dir)
}

func writeBootstrapProfiles(dir string, state *BootstrapState, opts BootstrapRitualOptions) error {
	name := strings.TrimSpace(opts.AgentName)
	if name == "" {
		name = "AnyClaw"
	}
	description := strings.TrimSpace(opts.AgentDescription)
	if description == "" {
		description = "Execution-oriented local AI assistant."
	}

	identityBlock := fmt.Sprintf(`## Bootstrap Identity

- Agent: %s
- Base description: %s
- Workspace focus: %s
- Default behavior: %s`, name, description, answerOrPlaceholder(state, "workspace_focus"), answerOrPlaceholder(state, "assistant_style"))

	userBlock := fmt.Sprintf(`## Bootstrap Preferences

- User profile: %s
- Constraints and boundaries: %s`, answerOrPlaceholder(state, "user_profile"), answerOrPlaceholder(state, "constraints"))

	soulBlock := fmt.Sprintf(`## Bootstrap Soul

- Mission: Help with %s
- Style: %s
- Boundaries: %s`, answerOrPlaceholder(state, "workspace_focus"), answerOrPlaceholder(state, "assistant_style"), answerOrPlaceholder(state, "constraints"))

	if err := upsertManagedMarkdownBlock(filepath.Join(dir, "IDENTITY.md"), identityBlock); err != nil {
		return err
	}
	if err := upsertManagedMarkdownBlock(filepath.Join(dir, "USER.md"), userBlock); err != nil {
		return err
	}
	if err := upsertManagedMarkdownBlock(filepath.Join(dir, "SOUL.md"), soulBlock); err != nil {
		return err
	}
	return nil
}

func answerOrPlaceholder(state *BootstrapState, id string) string {
	if state == nil || state.Answers == nil {
		return "(not provided)"
	}
	value := strings.TrimSpace(state.Answers[id])
	if value == "" {
		return "(not provided)"
	}
	return value
}

func upsertManagedMarkdownBlock(path string, block string) error {
	existingBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := normalizeNewlines(string(existingBytes))
	block = strings.TrimSpace(block)
	if block == "" {
		return nil
	}

	const startMarker = "<!-- anyclaw:bootstrap:start -->"
	const endMarker = "<!-- anyclaw:bootstrap:end -->"
	managed := startMarker + "\n" + block + "\n" + endMarker

	if strings.Contains(existing, startMarker) && strings.Contains(existing, endMarker) {
		start := strings.Index(existing, startMarker)
		end := strings.Index(existing, endMarker)
		if start >= 0 && end > start {
			end += len(endMarker)
			existing = strings.TrimSpace(existing[:start]) + "\n\n" + managed
			if end < len(normalizeNewlines(string(existingBytes))) {
				rest := strings.TrimSpace(normalizeNewlines(string(existingBytes))[end:])
				if rest != "" {
					existing += "\n\n" + rest
				}
			}
			return os.WriteFile(path, []byte(strings.TrimSpace(existing)+"\n"), 0o644)
		}
	}

	if strings.TrimSpace(existing) == "" {
		existing = managed
	} else {
		existing = strings.TrimSpace(existing) + "\n\n" + managed
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(existing)+"\n"), 0o644)
}

func loadBootstrapState(dir string) (*BootstrapState, error) {
	path := bootstrapStatePath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultBootstrapState(), nil
		}
		return nil, err
	}

	state := &BootstrapState{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}
	if state.Answers == nil {
		state.Answers = map[string]string{}
	}
	return state, nil
}

func saveBootstrapState(dir string, state *BootstrapState) error {
	if state == nil {
		state = defaultBootstrapState()
	}
	path := bootstrapStatePath(dir)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func clearBootstrapState(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.Remove(bootstrapStatePath(dir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func defaultBootstrapState() *BootstrapState {
	return &BootstrapState{
		Version:      1,
		CurrentIndex: 0,
		Answers:      map[string]string{},
	}
}

func bootstrapStatePath(dir string) string {
	return filepath.Join(strings.TrimSpace(dir), bootstrapStateFilename)
}
