package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBannerViewRespectsTerminalWidth(t *testing.T) {
	t.Setenv("COLUMNS", "64")

	rendered := bannerView("2026.3.13")
	if width := maxLineWidth(rendered); width > 64 {
		t.Fatalf("expected banner width <= 64, got %d\n%s", width, rendered)
	}
	if !strings.Contains(rendered, "AnyClaw") {
		t.Fatalf("expected banner to include title, got %q", rendered)
	}
}

func TestChatBodyWrapsWithinTerminalWidth(t *testing.T) {
	t.Setenv("COLUMNS", "44")

	rendered := ChatBody("这是一段很长很长很长很长的中文内容，用来验证聊天内容会在窄终端里自动换行。")
	if width := maxLineWidth(rendered); width > 44 {
		t.Fatalf("expected chat body width <= 44, got %d\n%s", width, rendered)
	}
	if !strings.Contains(rendered, "\n") {
		t.Fatalf("expected wrapped chat body to span multiple lines, got %q", rendered)
	}
}

func maxLineWidth(text string) int {
	lines := strings.Split(text, "\n")
	maxWidth := 0
	for _, line := range lines {
		if width := lipgloss.Width(line); width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}
