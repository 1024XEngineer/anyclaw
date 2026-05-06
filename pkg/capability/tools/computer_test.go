package tools

import (
	"context"
	"strings"
	"testing"
)

func TestComputerCoordinateDenormalization(t *testing.T) {
	tests := []struct {
		name   string
		value  int
		origin int
		size   int
		want   int
	}{
		{name: "middle", value: 500, origin: 10, size: 200, want: 110},
		{name: "clamps low", value: -10, origin: -100, size: 300, want: -100},
		{name: "clamps high to last pixel", value: 1500, origin: -100, size: 300, want: 199},
		{name: "right edge stays in bounds", value: 1000, origin: 10, size: 200, want: 209},
		{name: "zero size", value: 500, origin: 42, size: 0, want: 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := denormalizeComputerCoordinate(tt.value, tt.origin, tt.size); got != tt.want {
				t.Fatalf("denormalizeComputerCoordinate() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeComputerAction(t *testing.T) {
	tests := map[string]string{
		"click":            "click_at",
		"double-click":     "double_click_at",
		"hover":            "hover_at",
		"type_text":        "type_text_at",
		"scroll":           "scroll_document",
		"back":             "go_back",
		"forward":          "go_forward",
		"key_combination":  "key_combination",
		"drag_and_drop":    "drag_and_drop",
		"open_web_browser": "open_web_browser",
	}
	for input, want := range tests {
		if got := normalizeComputerAction(input); got != want {
			t.Fatalf("normalizeComputerAction(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeCodexComputerAction(t *testing.T) {
	tests := map[string]string{
		"click":           "click",
		"left-click":      "click",
		"double-click":    "double_click",
		"hover":           "move",
		"type_text":       "type",
		"key_combination": "keypress",
		"drag-and-drop":   "drag",
		"navigate":        "open_url",
		"search":          "search",
	}
	for input, want := range tests {
		if got := normalizeCodexComputerAction(input); got != want {
			t.Fatalf("normalizeCodexComputerAction(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestComputerActionKeys(t *testing.T) {
	keys, err := computerActionKeys("ctrl+shift+p")
	if err != nil {
		t.Fatalf("computerActionKeys string: %v", err)
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key.(string))
	}
	if got := strings.Join(parts, "+"); got != "ctrl+shift+p" {
		t.Fatalf("unexpected key split: %q", got)
	}

	keys, err = computerActionKeys([]string{"alt", "left"})
	if err != nil {
		t.Fatalf("computerActionKeys []string: %v", err)
	}
	if len(keys) != 2 || keys[0] != "alt" || keys[1] != "left" {
		t.Fatalf("unexpected keys: %#v", keys)
	}
}

func TestParseComputerActionRequestSupportsCodexActionBatch(t *testing.T) {
	req, err := parseComputerActionRequest(map[string]any{
		"actions": []any{
			map[string]any{"type": "click", "x": float64(500), "y": float64(250)},
			map[string]any{"type": "type", "text": "hello", "submit": true},
			map[string]any{"type": "keypress", "keys": []any{"ENTER"}},
		},
		"include_windows": false,
	}, ComputerOptions{MaxActionsPerTurn: 8, ObserveAfterAction: true, IncludeWindowsDefault: true})
	if err != nil {
		t.Fatalf("parseComputerActionRequest: %v", err)
	}
	if len(req.Actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(req.Actions))
	}
	if req.Actions[0].Type != "click" || !req.Actions[0].hasX || !req.Actions[0].hasY {
		t.Fatalf("unexpected first action: %#v", req.Actions[0])
	}
	if req.Actions[1].Type != "type" || req.Actions[1].Text != "hello" || !req.Actions[1].Submit {
		t.Fatalf("unexpected second action: %#v", req.Actions[1])
	}
	if req.Actions[2].Type != "keypress" || len(req.Actions[2].Keys) != 1 || req.Actions[2].Keys[0] != "ENTER" {
		t.Fatalf("unexpected third action: %#v", req.Actions[2])
	}
	if req.IncludeWindows {
		t.Fatal("explicit include_windows=false should override default true")
	}
	if !req.ObserveAfterAction {
		t.Fatal("expected observe_after_action default true")
	}
}

func TestParseComputerActionRequestInheritsConfiguredCoordinateSpace(t *testing.T) {
	req, err := parseComputerActionRequest(map[string]any{
		"actions": []any{
			map[string]any{"type": "click", "x": 10, "y": 20},
			map[string]any{"type": "click", "x": 500, "y": 500, "coordinate_space": "normalized_0_1000"},
		},
	}, ComputerOptions{CoordinateSpace: "absolute"})
	if err != nil {
		t.Fatalf("parseComputerActionRequest: %v", err)
	}
	if req.Actions[0].CoordinateSpace != "absolute" {
		t.Fatalf("expected first action to inherit absolute coordinate space, got %q", req.Actions[0].CoordinateSpace)
	}
	if req.Actions[1].CoordinateSpace != "normalized_0_1000" {
		t.Fatalf("expected explicit coordinate space to win, got %q", req.Actions[1].CoordinateSpace)
	}

	req, err = parseComputerActionRequest(map[string]any{
		"action": "click_at",
		"x":      10,
		"y":      20,
	}, ComputerOptions{CoordinateSpace: "pixels"})
	if err != nil {
		t.Fatalf("parse legacy computer action: %v", err)
	}
	if req.Actions[0].CoordinateSpace != "pixels" {
		t.Fatalf("expected legacy action to inherit pixels coordinate space, got %q", req.Actions[0].CoordinateSpace)
	}
}

func TestParseComputerActionRequestConvertsLegacyTypeTextAt(t *testing.T) {
	req, err := parseComputerActionRequest(map[string]any{
		"action":              "type_text_at",
		"x":                   500,
		"y":                   600,
		"text":                "hello",
		"clear_before_typing": true,
		"press_enter":         true,
	}, ComputerOptions{MaxActionsPerTurn: 8})
	if err != nil {
		t.Fatalf("parseComputerActionRequest: %v", err)
	}
	if len(req.Actions) != 3 {
		t.Fatalf("expected click, keypress, type actions, got %#v", req.Actions)
	}
	if req.Actions[0].Type != "click" || req.Actions[1].Type != "keypress" || req.Actions[2].Type != "type" {
		t.Fatalf("unexpected actions: %#v", req.Actions)
	}
	if req.Actions[2].Text != "hello" || !req.Actions[2].Submit {
		t.Fatalf("unexpected type action: %#v", req.Actions[2])
	}
}

func TestComputerControllerRejectsTooManyActionsBeforeDesktopAccess(t *testing.T) {
	controller := NewComputerController(BuiltinOptions{Computer: ComputerOptions{MaxActionsPerTurn: 1}})
	_, err := controller.Act(context.Background(), ComputerActionRequest{
		Actions: []ComputerAction{{Type: "wait", WaitMS: 1}, {Type: "wait", WaitMS: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "too many computer actions") {
		t.Fatalf("expected too many actions error, got %v", err)
	}
}

func TestComputerControllerRejectsUnsupportedLegacyBackend(t *testing.T) {
	controller := NewComputerController(BuiltinOptions{Computer: ComputerOptions{
		Backend:           "legacy_windows",
		MaxActionsPerTurn: 1,
	}})
	_, err := controller.Act(context.Background(), ComputerActionRequest{
		Actions: []ComputerAction{{Type: "wait", WaitMS: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported computer.backend") {
		t.Fatalf("expected unsupported backend error, got %v", err)
	}
}

func TestComputerEnabledDefaultsToTrueForZeroValueOptions(t *testing.T) {
	if !computerEnabled(ComputerOptions{}) {
		t.Fatal("zero-value computer options should preserve legacy enabled behavior")
	}
	if computerEnabled(ComputerOptions{Enabled: false, Backend: "codex_local"}) {
		t.Fatal("explicit disabled computer config should disable computer control")
	}
}

func TestComputerWindowAllowedMatchesProcessOrTitle(t *testing.T) {
	window := desktopWindowInfo{Title: "Untitled - Notepad", ProcessName: "notepad"}
	if !computerWindowAllowed(window, []string{"notepad.exe"}) {
		t.Fatal("expected process allowlist match")
	}
	if !computerWindowAllowed(window, []string{"Untitled"}) {
		t.Fatal("expected title allowlist match")
	}
	if computerWindowAllowed(window, []string{"chrome"}) {
		t.Fatal("did not expect unrelated app to match")
	}
}

func TestComputerPointProvidedSupportsExplicitZeroCoordinates(t *testing.T) {
	if !computerPointProvided(ComputerAction{X: 0, Y: 0, HasX: true, HasY: true}) {
		t.Fatal("expected explicit public zero coordinates to be treated as provided")
	}
	if computerPointProvided(ComputerAction{X: 0, Y: 0}) {
		t.Fatal("zero value coordinates without presence markers should remain missing")
	}
	if !computerDestinationProvided(ComputerAction{DestinationX: 0, DestinationY: 0, HasDestinationX: true, HasDestinationY: true}) {
		t.Fatal("expected explicit public zero destination to be treated as provided")
	}
}

func TestComputerDomainAllowedMatchesHostAndSubdomain(t *testing.T) {
	if !computerDomainAllowed("https://docs.example.com/path", []string{"example.com"}) {
		t.Fatal("expected parent domain to allow subdomain")
	}
	if !computerDomainAllowed("https://example.com/path", []string{".example.com"}) {
		t.Fatal("expected dotted allowlist entry to allow exact domain")
	}
	if computerDomainAllowed("https://evil-example.com", []string{"example.com"}) {
		t.Fatal("did not expect suffix lookalike domain to match")
	}
}

func TestComputerControllerBlocksDisallowedURLDomainBeforeDesktopAccess(t *testing.T) {
	controller := NewComputerController(BuiltinOptions{
		Computer: ComputerOptions{
			AllowedDomains:    []string{"example.com"},
			MaxActionsPerTurn: 1,
		},
	})
	_, err := controller.Act(context.Background(), ComputerActionRequest{
		Actions: []ComputerAction{{Type: "open_url", URL: "https://openai.com"}},
	})
	if err == nil || !strings.Contains(err.Error(), "blocked for URL domain") {
		t.Fatalf("expected disallowed URL domain error, got %v", err)
	}
}

func TestComputerControllerBlocksURLActionsWhenAppAllowlistIsSet(t *testing.T) {
	controller := NewComputerController(BuiltinOptions{
		Computer: ComputerOptions{
			AllowedApps:       []string{"notepad"},
			AllowedDomains:    []string{"example.com"},
			MaxActionsPerTurn: 1,
		},
	})
	_, err := controller.Act(context.Background(), ComputerActionRequest{
		Actions: []ComputerAction{{Type: "open_url", URL: "https://example.com"}},
	})
	if err == nil || !strings.Contains(err.Error(), "blocked while computer.allowed_apps is set") {
		t.Fatalf("expected allowed_apps URL block before desktop access, got %v", err)
	}
}

func TestSanitizeComputerAuditInputRedactsText(t *testing.T) {
	sanitized := sanitizeComputerAuditInput(map[string]any{
		"action": "type_text_at",
		"text":   "secret text",
		"actions": []any{
			map[string]any{"type": "type", "text": "hello"},
			map[string]any{"type": "click", "x": 10, "y": 20},
		},
		"api_token": "abc123",
	})
	if got := sanitized["text"]; got != "[REDACTED len=11]" {
		t.Fatalf("expected top-level text redaction, got %#v", got)
	}
	if got := sanitized["api_token"]; got != "[REDACTED]" {
		t.Fatalf("expected token redaction, got %#v", got)
	}
	actions, ok := sanitized["actions"].([]any)
	if !ok || len(actions) != 2 {
		t.Fatalf("expected sanitized actions, got %#v", sanitized["actions"])
	}
	first, ok := actions[0].(map[string]any)
	if !ok || first["text"] != "[REDACTED len=5]" {
		t.Fatalf("expected nested text redaction, got %#v", actions[0])
	}
}

func TestSanitizeComputerAuditInputHandlesTypedActionMaps(t *testing.T) {
	sanitized := sanitizeComputerAuditInput(map[string]any{
		"actions": []map[string]any{
			{"type": "type", "text": "hello"},
		},
	})
	actions, ok := sanitized["actions"].([]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("expected typed action maps to sanitize into []any, got %#v", sanitized["actions"])
	}
	first, ok := actions[0].(map[string]any)
	if !ok || first["text"] != "[REDACTED len=5]" {
		t.Fatalf("expected typed action text redaction, got %#v", actions[0])
	}
}

func TestRegisterBuiltinsAddsComputerUseTools(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{})

	for _, name := range []string{"computer_observe", "computer_action"} {
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("expected %s to be registered", name)
		}
		if tool.Category != ToolCategoryDesktop {
			t.Fatalf("expected %s category desktop, got %q", name, tool.Category)
		}
		if !tool.RequiresApproval {
			t.Fatalf("expected %s to require approval", name)
		}
		if tool.CachePolicy != ToolCachePolicyNever {
			t.Fatalf("expected %s to avoid cache, got %q", name, tool.CachePolicy)
		}
	}
	tool, _ := registry.Get("computer_action")
	properties, _ := tool.InputSchema["properties"].(map[string]any)
	if _, ok := properties["actions"]; !ok {
		t.Fatalf("expected computer_action schema to expose actions batch, got %#v", tool.InputSchema)
	}
}

func TestComputerToolsRequestApprovalBeforeDesktopAccess(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, BuiltinOptions{
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
	})

	var called ToolApprovalCall
	ctx := WithToolApprovalHook(context.Background(), func(ctx context.Context, call ToolApprovalCall) error {
		called = call
		return context.Canceled
	})

	_, err := registry.Call(ctx, "computer_observe", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected approval hook error, got %v", err)
	}
	if called.Name != "computer_observe" {
		t.Fatalf("expected computer_observe approval, got %#v", called)
	}

	called = ToolApprovalCall{}
	_, err = registry.Call(ctx, "computer_action", map[string]any{"action": "wait"})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected approval hook error, got %v", err)
	}
	if called.Name != "computer_action" {
		t.Fatalf("expected computer_action approval, got %#v", called)
	}
}

func TestComputerActionToolValidatesInputBeforeDesktopAccess(t *testing.T) {
	_, err := ComputerActionTool(context.Background(), map[string]any{}, BuiltinOptions{
		ExecutionMode:   "sandbox",
		PermissionLevel: "full",
		Computer:        ComputerOptions{Enabled: true, Backend: "codex_local"},
	})
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Fatalf("expected input validation error before desktop access error, got %v", err)
	}
}
