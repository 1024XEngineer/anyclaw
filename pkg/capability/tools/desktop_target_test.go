package tools

import "testing"

func TestResolveDesktopTargetStrategiesAuto(t *testing.T) {
	strategies, err := resolveDesktopTargetStrategies(map[string]any{
		"title":         "QQ",
		"name":          "发送",
		"text":          "发送",
		"template_path": "send-button.png",
	})
	if err != nil {
		t.Fatalf("resolveDesktopTargetStrategies: %v", err)
	}
	expected := []string{"ui", "text", "image", "window"}
	if len(strategies) != len(expected) {
		t.Fatalf("expected %d strategies, got %d: %#v", len(expected), len(strategies), strategies)
	}
	for i, item := range expected {
		if strategies[i] != item {
			t.Fatalf("expected strategy %d to be %q, got %q", i, item, strategies[i])
		}
	}
}

func TestResolveDesktopTargetStrategiesExplicit(t *testing.T) {
	strategies, err := resolveDesktopTargetStrategies(map[string]any{
		"strategies": []any{"image", "text", "image"},
	})
	if err != nil {
		t.Fatalf("resolveDesktopTargetStrategies: %v", err)
	}
	expected := []string{"image", "text"}
	if len(strategies) != len(expected) {
		t.Fatalf("expected %d strategies, got %d: %#v", len(expected), len(strategies), strategies)
	}
	for i, item := range expected {
		if strategies[i] != item {
			t.Fatalf("expected strategy %d to be %q, got %q", i, item, strategies[i])
		}
	}
}

func TestResolveDesktopTargetStrategiesRequireSelector(t *testing.T) {
	_, err := resolveDesktopTargetStrategies(map[string]any{})
	if err == nil {
		t.Fatal("expected empty selector set to be rejected")
	}
}

func TestShouldCropDesktopTargetToWindowDefaults(t *testing.T) {
	window := &desktopWindowInfo{X: 10, Y: 20, Width: 400, Height: 300}
	if !shouldCropDesktopTargetToWindow(map[string]any{}, window) {
		t.Fatal("expected captured desktop source to crop to the target window by default")
	}
	if shouldCropDesktopTargetToWindow(map[string]any{"path": "shot.png"}, window) {
		t.Fatal("expected explicit path source to skip implicit cropping")
	}
	if !shouldCropDesktopTargetToWindow(map[string]any{"path": "shot.png", "crop_to_window": true}, window) {
		t.Fatal("expected crop_to_window override to force cropping")
	}
}

func TestApplyDesktopTargetOffsets(t *testing.T) {
	textMatch := ocrTextMatchResult{X: 5, Y: 6, CenterX: 15, CenterY: 16}
	applyTextMatchOffset(&textMatch, 100, 200)
	if textMatch.X != 105 || textMatch.Y != 206 || textMatch.CenterX != 115 || textMatch.CenterY != 216 {
		t.Fatalf("unexpected text match offset result: %#v", textMatch)
	}

	imageMatch := imageMatchResult{X: 7, Y: 8, CenterX: 17, CenterY: 18}
	applyImageMatchOffset(&imageMatch, 50, 70)
	if imageMatch.X != 57 || imageMatch.Y != 78 || imageMatch.CenterX != 67 || imageMatch.CenterY != 88 {
		t.Fatalf("unexpected image match offset result: %#v", imageMatch)
	}
}
