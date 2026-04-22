package contextengine

import (
	"testing"
)

func TestWindowGuardBasic(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	if g.MaxTokens() != 1000 {
		t.Errorf("expected max tokens 1000, got %d", g.MaxTokens())
	}

	if err := g.Add(500); err != nil {
		t.Fatalf("add 500: %v", err)
	}

	if g.CurrentTokens() != 500 {
		t.Errorf("expected 500 tokens, got %d", g.CurrentTokens())
	}

	used, max, avail := g.Status()
	if used != 500 || max != 1000 {
		t.Errorf("expected used=500, max=1000, got used=%d, max=%d", used, max)
	}
	if avail < 449 || avail > 451 {
		t.Errorf("expected available ~450, got %d", avail)
	}
}

func TestWindowGuardExceed(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	g.Add(500)

	if err := g.Add(600); err == nil {
		t.Error("expected error when exceeding limit")
	}
}

func TestWindowGuardHardLimit(t *testing.T) {
	g := NewWindowGuard(GuardConfig{
		MaxTokens:    1000,
		SafetyMargin: 0,
		HardLimit:    true,
	})

	if err := g.Add(1001); err == nil {
		t.Error("expected error for hard limit exceed")
	}

	if err := g.Add(1000); err != nil {
		t.Fatalf("expected no error at exact limit, got %v", err)
	}
}

func TestWindowGuardCheck(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	g.Add(400)

	if !g.Check(500) {
		t.Error("expected check to pass")
	}

	if g.Check(600) {
		t.Error("expected check to fail")
	}
}

func TestWindowGuardRemove(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	g.Add(500)
	g.Remove(200)

	if g.CurrentTokens() != 300 {
		t.Errorf("expected 300 tokens after remove, got %d", g.CurrentTokens())
	}
}

func TestWindowGuardRemoveUnderflow(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	g.Add(100)
	g.Remove(200)

	if g.CurrentTokens() != 0 {
		t.Errorf("expected 0 tokens after underflow remove, got %d", g.CurrentTokens())
	}
}

func TestWindowGuardReset(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	g.Add(500)
	g.Reset()

	if g.CurrentTokens() != 0 {
		t.Errorf("expected 0 tokens after reset, got %d", g.CurrentTokens())
	}
}

func TestWindowGuardSetMaxTokens(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	g.SetMaxTokens(2000)
	if g.MaxTokens() != 2000 {
		t.Errorf("expected max tokens 2000, got %d", g.MaxTokens())
	}
}

func TestWindowGuardSafetyMargin(t *testing.T) {
	g := NewWindowGuard(GuardConfig{
		MaxTokens:    1000,
		SafetyMargin: 100,
		HardLimit:    false,
	})

	if err := g.Add(850); err != nil {
		t.Fatalf("add 850: %v", err)
	}

	if err := g.Add(100); err == nil {
		t.Error("expected error when exceeding with safety margin")
	}

	if err := g.Add(50); err != nil {
		t.Fatalf("add 50 should succeed: %v", err)
	}
}

func TestSimpleTokenEstimator(t *testing.T) {
	est := SimpleTokenEstimator(4)

	tokens := est("Hello world this is a test")
	if tokens != 6 {
		t.Errorf("expected 6 tokens, got %d", tokens)
	}

	est2 := SimpleTokenEstimator(1)
	tokens2 := est2("test")
	if tokens2 != 4 {
		t.Errorf("expected 4 tokens with 1 rune/token, got %d", tokens2)
	}
}

type mockTrimmable struct {
	tokens int
	pinned bool
}

func (m *mockTrimmable) TokenCount() int { return m.tokens }
func (m *mockTrimmable) IsPinned() bool  { return m.pinned }

func TestWindowGuardCalculateTrim(t *testing.T) {
	g := NewWindowGuard(GuardConfig{
		MaxTokens:    1000,
		SafetyMargin: 50,
	})
	g.Add(900)

	items := []Trimmable{
		&mockTrimmable{tokens: 50, pinned: true},
		&mockTrimmable{tokens: 100},
		&mockTrimmable{tokens: 200},
	}

	trim := g.CalculateTrim(items, 100)
	if trim < 50 {
		t.Errorf("expected trim >= 50, got %d", trim)
	}
}

func TestWindowGuardCalculateTrimNoExcess(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))
	g.Add(100)

	items := []Trimmable{
		&mockTrimmable{tokens: 100},
	}

	trim := g.CalculateTrim(items, 50)
	if trim != 0 {
		t.Errorf("expected no trim needed, got %d", trim)
	}
}

func TestWindowGuardAvailableTokens(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(1000))

	if g.AvailableTokens() < 949 || g.AvailableTokens() > 951 {
		t.Errorf("expected available ~950, got %d", g.AvailableTokens())
	}

	g.Add(500)
	if g.AvailableTokens() < 449 || g.AvailableTokens() > 451 {
		t.Errorf("expected available ~450, got %d", g.AvailableTokens())
	}
}

func TestWindowGuardDefaultConfig(t *testing.T) {
	cfg := DefaultGuardConfig(4096)
	if cfg.MaxTokens != 4096 {
		t.Errorf("expected max tokens 4096, got %d", cfg.MaxTokens)
	}
	if cfg.SafetyMargin != 204 {
		t.Errorf("expected safety margin 204, got %d", cfg.SafetyMargin)
	}
}

func TestWindowGuardZeroConfig(t *testing.T) {
	g := NewWindowGuard(GuardConfig{})

	if g.MaxTokens() != 4096 {
		t.Errorf("expected default max tokens 4096, got %d", g.MaxTokens())
	}
}

func TestWindowGuardConcurrent(t *testing.T) {
	g := NewWindowGuard(DefaultGuardConfig(10000))

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			g.Add(10)
			g.Remove(5)
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if g.CurrentTokens() != 500 {
		t.Errorf("expected 500 tokens, got %d", g.CurrentTokens())
	}
}
