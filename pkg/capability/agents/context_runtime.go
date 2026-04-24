package agent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/prompt"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	bootstrapwatch "github.com/1024XEngineer/anyclaw/pkg/runtime/context/bootstrapwatch"
	ctxpkg "github.com/1024XEngineer/anyclaw/pkg/runtime/context/store"
	ctxengine "github.com/1024XEngineer/anyclaw/pkg/runtime/context/window"
	"github.com/1024XEngineer/anyclaw/pkg/workspace"
)

const (
	defaultContextMaxTokens     = 4096
	defaultRecentHistoryToKeep  = 4
	defaultBootstrapCharsFactor = 8
)

type contextRuntime struct {
	slot              *ctxengine.ExclusiveSlot
	estimator         ctxengine.TokenEstimator
	maxTokens         int
	safetyMargin      int
	bootstrapMaxRunes int

	contextEngine ctxpkg.ContextEngine

	bootstrapMu    sync.RWMutex
	bootstrapCache []workspace.BootstrapFile
	bootstrapDirty bool
	bootstrapWatch *bootstrapwatch.Watcher
	executionSeq   atomic.Uint64
}

type contextExecution struct {
	id string
}

func newContextRuntime(cfg Config) *contextRuntime {
	maxTokens := cfg.MaxContextTokens
	if maxTokens <= 0 {
		maxTokens = defaultContextMaxTokens
	}

	safetyMargin := cfg.ContextSafetyMargin
	if safetyMargin < 0 {
		safetyMargin = 0
	}
	if safetyMargin == 0 {
		safetyMargin = ctxengine.DefaultGuardConfig(maxTokens).SafetyMargin
	}

	slotCfg := ctxengine.DefaultSlotConfig()
	slotCfg.MaxIdle = 30 * time.Minute
	slotCfg.MaxDuration = 2 * time.Hour

	runtime := &contextRuntime{
		slot:              ctxengine.NewExclusiveSlot(slotCfg),
		estimator:         ctxengine.SimpleTokenEstimator(4),
		maxTokens:         maxTokens,
		safetyMargin:      safetyMargin,
		bootstrapMaxRunes: maxTokens * defaultBootstrapCharsFactor,
		contextEngine:     normalizeContextEngine(cfg.ContextEngine),
		bootstrapDirty:    true,
	}

	if runtime.bootstrapMaxRunes <= 0 || runtime.bootstrapMaxRunes > workspace.DefaultBootstrapMaxChars {
		runtime.bootstrapMaxRunes = workspace.DefaultBootstrapMaxChars
	}

	runtime.startBootstrapWatcher(cfg.WorkingDir, cfg.BootstrapWatchInterval)
	return runtime
}

func (r *contextRuntime) startBootstrapWatcher(workingDir string, interval time.Duration) {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return
	}

	cfg := bootstrapwatch.DefaultWatcherConfig(workingDir)
	if interval > 0 {
		cfg.PollInterval = interval
	}
	cfg.Files = bootstrapWatcherFiles()
	cfg.OnChange = func(event bootstrapwatch.ChangeEvent) {
		r.bootstrapMu.Lock()
		r.bootstrapDirty = true
		r.bootstrapMu.Unlock()
	}

	watcher := bootstrapwatch.NewWatcher(cfg)
	if err := watcher.Start(); err != nil {
		return
	}

	r.bootstrapMu.Lock()
	r.bootstrapWatch = watcher
	r.bootstrapMu.Unlock()
}

func bootstrapWatcherFiles() []bootstrapwatch.FileType {
	return []bootstrapwatch.FileType{
		bootstrapwatch.FileAgents,
		bootstrapwatch.FileSoul,
		bootstrapwatch.FileTools,
		bootstrapwatch.FileIdentity,
		bootstrapwatch.FileUser,
		bootstrapwatch.FileHeartbeat,
		bootstrapwatch.FileBootstrap,
		bootstrapwatch.FileMemory,
	}
}

func (r *contextRuntime) acquire(ctx context.Context, owner string) (*contextExecution, error) {
	if r == nil || r.slot == nil {
		return &contextExecution{}, nil
	}

	id := fmt.Sprintf("%s-%d-%d", sanitizeBrowserSessionID(owner), time.Now().UnixNano(), r.executionSeq.Add(1))
	result, err := r.slot.Acquire(ctx, id, ctxengine.ContextConfig{
		MaxAge:          2 * time.Hour,
		AutoExpire:      true,
		CleanupInterval: time.Minute,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Granted {
		if result != nil && result.Error != nil {
			return nil, result.Error
		}
		return nil, fmt.Errorf("context slot was not granted")
	}
	return &contextExecution{id: id}, nil
}

func (r *contextRuntime) heartbeat(exec *contextExecution) {
	if r == nil || r.slot == nil || exec == nil || strings.TrimSpace(exec.id) == "" {
		return
	}
	_ = r.slot.Heartbeat(exec.id)
}

func (r *contextRuntime) release(exec *contextExecution) {
	if r == nil || r.slot == nil || exec == nil || strings.TrimSpace(exec.id) == "" {
		return
	}
	r.slot.Release(exec.id)
}

func (r *contextRuntime) setContextEngine(engine ctxpkg.ContextEngine) {
	if r == nil {
		return
	}
	r.contextEngine = normalizeContextEngine(engine)
}

func (r *contextRuntime) storeConversation(ctx context.Context, agentName string, workingDir string, role string, content string) {
	if r == nil || normalizeContextEngine(r.contextEngine) == nil {
		return
	}

	content = strings.TrimSpace(content)
	role = strings.TrimSpace(role)
	if content == "" || role == "" {
		return
	}

	doc := ctxpkg.Document{
		ID:      fmt.Sprintf("%s_%d", role, time.Now().UnixNano()),
		Content: content,
		Metadata: map[string]any{
			"role":        role,
			"agent_name":  agentName,
			"working_dir": workingDir,
			"source":      "agent.run",
		},
	}
	_ = r.contextEngine.AddDocument(ctx, doc)
}

func normalizeContextEngine(engine ctxpkg.ContextEngine) ctxpkg.ContextEngine {
	if engine == nil {
		return nil
	}
	value := reflect.ValueOf(engine)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil
		}
	}
	return engine
}

func (r *contextRuntime) loadBootstrapFiles(workingDir string) ([]workspace.BootstrapFile, error) {
	if r == nil {
		return nil, nil
	}

	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return nil, nil
	}

	r.bootstrapMu.RLock()
	if !r.bootstrapDirty && len(r.bootstrapCache) > 0 {
		files := copyBootstrapFiles(r.bootstrapCache)
		r.bootstrapMu.RUnlock()
		return files, nil
	}
	r.bootstrapMu.RUnlock()

	files, err := workspace.LoadBootstrapFiles(workingDir, workspace.BootstrapOptions{
		BootstrapMaxChars: r.bootstrapMaxRunes,
	})
	if err != nil {
		return nil, err
	}

	r.bootstrapMu.Lock()
	r.bootstrapCache = copyBootstrapFiles(files)
	r.bootstrapDirty = false
	r.bootstrapMu.Unlock()
	return files, nil
}

func copyBootstrapFiles(files []workspace.BootstrapFile) []workspace.BootstrapFile {
	if len(files) == 0 {
		return nil
	}
	cloned := make([]workspace.BootstrapFile, len(files))
	copy(cloned, files)
	return cloned
}

func (r *contextRuntime) compactHistory(ctx context.Context, history []prompt.Message, llmCaller LLMCaller) ([]prompt.Message, error) {
	if r == nil || len(history) == 0 {
		return history, nil
	}

	guard := ctxengine.NewWindowGuard(ctxengine.GuardConfig{
		MaxTokens:    r.maxTokens,
		SafetyMargin: r.safetyMargin,
		HardLimit:    false,
	})
	compactor := ctxengine.NewCompactor(guard, ctxengine.DefaultCompactionConfig())

	now := time.Now()
	for i, msg := range history {
		compactor.Add(&ctxengine.Message{
			ID:         fmt.Sprintf("history_%d", i),
			Role:       msg.Role,
			Content:    msg.Content,
			Tokens:     r.messageTokens(msg),
			Pinned:     msg.Role == "system" || i >= len(history)-defaultRecentHistoryToKeep,
			CreatedAt:  now.Add(time.Duration(i) * time.Millisecond),
			AccessedAt: now.Add(time.Duration(i) * time.Millisecond),
		})
	}

	if !compactor.NeedsCompaction() {
		return history, nil
	}

	_, err := compactor.Compact(ctx, llmSummarizer{llm: llmCaller})
	if err != nil {
		return history, nil
	}

	return toPromptMessages(compactor.Messages()), nil
}

func (r *contextRuntime) enforceTokenLimit(systemPrompt string, history []prompt.Message) ([]prompt.Message, error) {
	if r == nil {
		return history, nil
	}

	guard := ctxengine.NewWindowGuard(ctxengine.GuardConfig{
		MaxTokens:    r.maxTokens,
		SafetyMargin: r.safetyMargin,
		HardLimit:    true,
	})

	systemTokens := r.estimator(systemPrompt)
	if err := guard.Add(systemTokens); err != nil {
		return nil, fmt.Errorf("context window exceeded by system prompt: %w", err)
	}

	selected := make([]prompt.Message, 0, len(history))
	selectedSystem := make([]prompt.Message, 0, 2)
	for _, msg := range history {
		if msg.Role != "system" {
			continue
		}
		tokens := r.messageTokens(msg)
		if !guard.Check(tokens) {
			continue
		}
		if err := guard.Add(tokens); err != nil {
			continue
		}
		selectedSystem = append(selectedSystem, msg)
	}

	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		if msg.Role == "system" {
			continue
		}
		tokens := r.messageTokens(msg)
		if !guard.Check(tokens) {
			continue
		}
		if err := guard.Add(tokens); err != nil {
			continue
		}
		selected = append(selected, msg)
	}

	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}

	return append(selectedSystem, selected...), nil
}

func (r *contextRuntime) messageTokens(msg prompt.Message) int {
	if r == nil {
		return 0
	}
	return r.estimator(msg.Role) + r.estimator(msg.Content) + 4
}

func toPromptMessages(messages []*ctxengine.Message) []prompt.Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]prompt.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		result = append(result, prompt.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return result
}

type llmSummarizer struct {
	llm LLMCaller
}

func (s llmSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	if s.llm == nil {
		return "", fmt.Errorf("llm unavailable")
	}

	resp, err := s.llm.Chat(ctx, []llm.Message{
		{
			Role:    "system",
			Content: "You are a conversation summarizer. Summarize the following conversation concisely, preserving key facts, decisions, and pending work.",
		},
		{
			Role:    "user",
			Content: text,
		},
	}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}
