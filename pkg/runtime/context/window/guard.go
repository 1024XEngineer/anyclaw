package contextengine

import (
	"fmt"
	"sync"
)

type WindowGuard struct {
	mu            sync.Mutex
	maxTokens     int
	currentTokens int
	safetyMargin  int
	hardLimit     bool
}

type GuardConfig struct {
	MaxTokens    int
	SafetyMargin int
	HardLimit    bool
}

func DefaultGuardConfig(maxTokens int) GuardConfig {
	return GuardConfig{
		MaxTokens:    maxTokens,
		SafetyMargin: maxTokens / 20, // 5% safety margin
		HardLimit:    false,
	}
}

func NewWindowGuard(cfg GuardConfig) *WindowGuard {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.SafetyMargin < 0 {
		cfg.SafetyMargin = 0
	}
	return &WindowGuard{
		maxTokens:    cfg.MaxTokens,
		safetyMargin: cfg.SafetyMargin,
		hardLimit:    cfg.HardLimit,
	}
}

func (g *WindowGuard) MaxTokens() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.maxTokens
}

func (g *WindowGuard) SetMaxTokens(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.maxTokens = n
}

func (g *WindowGuard) CurrentTokens() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.currentTokens
}

func (g *WindowGuard) AvailableTokens() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	avail := g.maxTokens - g.currentTokens - g.safetyMargin
	if avail < 0 {
		return 0
	}
	return avail
}

func (g *WindowGuard) Status() (used, max, available int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	avail := g.maxTokens - g.currentTokens - g.safetyMargin
	if avail < 0 {
		avail = 0
	}
	return g.currentTokens, g.maxTokens, avail
}

func (g *WindowGuard) Check(tokens int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.currentTokens+tokens+g.safetyMargin <= g.maxTokens
}

func (g *WindowGuard) Add(tokens int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.hardLimit && g.currentTokens+tokens > g.maxTokens {
		return fmt.Errorf("window guard: hard limit exceeded (%d + %d > %d)", g.currentTokens, tokens, g.maxTokens)
	}

	if g.currentTokens+tokens+g.safetyMargin > g.maxTokens {
		return fmt.Errorf("window guard: limit exceeded with safety margin (%d + %d + %d > %d)",
			g.currentTokens, tokens, g.safetyMargin, g.maxTokens)
	}

	g.currentTokens += tokens
	return nil
}

func (g *WindowGuard) Remove(tokens int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.currentTokens -= tokens
	if g.currentTokens < 0 {
		g.currentTokens = 0
	}
}

func (g *WindowGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.currentTokens = 0
}

type TokenEstimator func(text string) int

func SimpleTokenEstimator(runesPerToken int) TokenEstimator {
	if runesPerToken <= 0 {
		runesPerToken = 4
	}
	return func(text string) int {
		return len([]rune(text)) / runesPerToken
	}
}

type Trimmable interface {
	TokenCount() int
	IsPinned() bool
}

func (g *WindowGuard) CalculateTrim(items []Trimmable, needed int) int {
	g.mu.Lock()
	current := g.currentTokens
	max := g.maxTokens
	margin := g.safetyMargin
	g.mu.Unlock()

	excess := current + needed + margin - max
	if excess <= 0 {
		return 0
	}

	removed := 0
	for _, item := range items {
		if item.IsPinned() {
			continue
		}
		removed += item.TokenCount()
		if removed >= excess {
			break
		}
	}

	return removed
}
