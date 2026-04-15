package agent

import (
	"context"
	"fmt"
	"time"

	"anyclaw/internal/hooks"
	"anyclaw/internal/providers"
	"anyclaw/internal/session"
	"anyclaw/internal/tools"
	"anyclaw/internal/util"
	"anyclaw/pkg/sdk"
)

// RunRequest is one agent execution request.
type RunRequest struct {
	RunID       string
	AgentID     string
	SessionKey  string
	UserMessage session.ChatMessage
	ProviderID  string
	Model       string
	Metadata    map[string]any
}

// Runner executes one full agent loop.
type Runner struct {
	Sessions  session.SessionStore
	Providers *providers.Registry
	Tools     *tools.Registry
	Hooks     *hooks.Registry
}

// NewRunner creates an agent runner with its core dependencies.
func NewRunner(store session.SessionStore, providerRegistry *providers.Registry, toolRegistry *tools.Registry, hookRegistry *hooks.Registry) *Runner {
	return &Runner{Sessions: store, Providers: providerRegistry, Tools: toolRegistry, Hooks: hookRegistry}
}

// Run executes the starter agent loop and streams lifecycle and assistant events.
func (r *Runner) Run(ctx context.Context, req RunRequest) (<-chan Event, error) {
	if req.RunID == "" {
		req.RunID = util.NewID()
	}
	stream := make(chan Event, 8)
	go func() {
		defer close(stream)
		stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "start", Data: map[string]any{"agentId": req.AgentID, "sessionKey": req.SessionKey}}

		sess, err := r.Sessions.GetOrCreate(ctx, req.SessionKey)
		if err != nil {
			stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "error", Data: map[string]any{"error": err.Error()}}
			return
		}

		if err := sess.Append(ctx, req.UserMessage); err != nil {
			stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "error", Data: map[string]any{"error": err.Error()}}
			return
		}

		history, err := sess.History(ctx, 50)
		if err != nil {
			stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "error", Data: map[string]any{"error": err.Error()}}
			return
		}

		hookContext := sdk.HookContext{RunID: req.RunID, AgentID: req.AgentID, SessionKey: req.SessionKey, Data: map[string]any{"model": req.Model, "providerId": req.ProviderID}}
		if err := RunHooks(ctx, r.Hooks, sdk.HookBeforeModelResolve, hookContext); err != nil {
			stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "error", Data: map[string]any{"error": err.Error()}}
			return
		}
		if err := RunHooks(ctx, r.Hooks, sdk.HookBeforePromptBuild, hookContext); err != nil {
			stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "error", Data: map[string]any{"error": err.Error()}}
			return
		}

		modelRequest := sdk.ModelRequest{
			Model:    req.Model,
			Messages: BuildPrompt(history[:max(len(history)-1, 0)], req.UserMessage),
			Metadata: req.Metadata,
		}
		for _, tool := range r.Tools.List() {
			modelRequest.Tools = append(modelRequest.Tools, tool.Spec())
		}

		executor := ModelExecutor{Providers: r.Providers}
		chunks, err := executor.Stream(ctx, req.ProviderID, modelRequest)
		if err != nil {
			stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "error", Data: map[string]any{"error": err.Error()}}
			return
		}

		var assistantText string
		for chunk := range chunks {
			if chunk.Text != "" {
				assistantText += chunk.Text
				stream <- Event{RunID: req.RunID, Type: EventAssistant, Name: "delta", Data: map[string]any{"text": chunk.Text}}
			}
			if chunk.Tool != nil {
				stream <- Event{RunID: req.RunID, Type: EventTool, Name: "call", Data: map[string]any{"tool": chunk.Tool.Name}}
			}
		}

		assistantMessage := session.ChatMessage{ID: util.NewID(), Role: "assistant", Text: assistantText, CreatedAt: time.Now()}
		if assistantText == "" {
			assistantMessage.Text = fmt.Sprintf("%s finished without assistant text", req.ProviderID)
		}
		if err := sess.Append(ctx, assistantMessage); err != nil {
			stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "error", Data: map[string]any{"error": err.Error()}}
			return
		}
		_ = RunHooks(ctx, r.Hooks, sdk.HookAgentEnd, hookContext)
		stream <- Event{RunID: req.RunID, Type: EventLifecycle, Name: "end", Data: map[string]any{"assistantText": assistantMessage.Text}}
	}()
	return stream, nil
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
