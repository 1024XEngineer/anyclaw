package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"
)

type desktopTargetResolution struct {
	Found    bool                      `json:"found"`
	Strategy string                    `json:"strategy,omitempty"`
	Action   string                    `json:"action,omitempty"`
	X        int                       `json:"x,omitempty"`
	Y        int                       `json:"y,omitempty"`
	Width    int                       `json:"width,omitempty"`
	Height   int                       `json:"height,omitempty"`
	CenterX  int                       `json:"center_x,omitempty"`
	CenterY  int                       `json:"center_y,omitempty"`
	Window   *desktopWindowInfo        `json:"window,omitempty"`
	Element  *desktopAutomationElement `json:"element,omitempty"`
	Text     *ocrTextMatchResult       `json:"text,omitempty"`
	Image    *imageMatchResult         `json:"image,omitempty"`
	Meta     map[string]any            `json:"meta,omitempty"`
}

type desktopVisionSource struct {
	Path    string
	OffsetX int
	OffsetY int
	Cleanup func()
}

func DesktopResolveTargetTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_resolve_target", opts, true); err != nil {
		return "", err
	}
	result, err := resolveDesktopTarget(ctx, input, opts)
	if err != nil {
		return "", err
	}
	if boolValue(input["require_found"]) && !result.Found {
		return "", desktopTargetNotFoundError(result)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopActivateTargetTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_activate_target", opts, false); err != nil {
		return "", err
	}
	result, err := resolveDesktopTarget(ctx, input, opts)
	if err != nil {
		return "", err
	}
	if !result.Found {
		return "", desktopTargetNotFoundError(result)
	}
	action, err := normalizeDesktopTargetAction(stringValue(input["action"]))
	if err != nil {
		return "", err
	}
	switch result.Strategy {
	case "ui":
		uiInput := cloneInputMap(input)
		switch action {
		case "auto", "click", "focus", "invoke", "select", "expand", "collapse", "toggle":
			uiInput["action"] = action
		case "double_click":
			if _, err := DesktopDoubleClickTool(ctx, map[string]any{
				"x": result.CenterX,
				"y": result.CenterY,
			}, opts); err != nil {
				return "", err
			}
			result.Action = "double_click"
		default:
			return "", fmt.Errorf("unsupported action for ui target: %s", action)
		}
		if result.Action == "" {
			if boolValue(input["human_like"]) && (action == "auto" || action == "click" || action == "invoke") {
				clickInput := map[string]any{
					"x":          result.CenterX,
					"y":          result.CenterY,
					"button":     firstNonEmpty(stringValue(input["button"]), "left"),
					"human_like": true,
				}
				for _, key := range []string{"duration_ms", "steps", "jitter_px", "settle_ms"} {
					if value, ok := input[key]; ok {
						clickInput[key] = value
					}
				}
				if _, err := DesktopClickTool(ctx, clickInput, opts); err != nil {
					return "", err
				}
				result.Action = "click"
			}
		}
		if result.Action == "" {
			raw, err := DesktopInvokeUITool(ctx, uiInput, opts)
			if err != nil {
				return "", err
			}
			var uiResult desktopAutomationActionResult
			if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &uiResult); err == nil {
				result.Action = uiResult.Action
				if uiResult.Element != nil {
					result.Element = uiResult.Element
					applyDesktopElementBounds(&result, *uiResult.Element)
				}
			} else {
				result.Action = action
				result.Meta = mergeMeta(result.Meta, map[string]any{"activation_result": raw})
			}
		}
	case "text", "image":
		if err := focusDesktopTargetWindow(ctx, input, result.Window, opts); err != nil {
			return "", err
		}
		switch action {
		case "focus":
			result.Action = "focus"
		case "auto", "click", "invoke":
			clickInput := map[string]any{
				"x":      result.CenterX,
				"y":      result.CenterY,
				"button": firstNonEmpty(stringValue(input["button"]), "left"),
			}
			for _, key := range []string{"human_like", "duration_ms", "steps", "jitter_px", "settle_ms"} {
				if value, ok := input[key]; ok {
					clickInput[key] = value
				}
			}
			if _, err := DesktopClickTool(ctx, clickInput, opts); err != nil {
				return "", err
			}
			result.Action = "click"
		case "double_click":
			doubleInput := map[string]any{
				"x":      result.CenterX,
				"y":      result.CenterY,
				"button": firstNonEmpty(stringValue(input["button"]), "left"),
			}
			for _, key := range []string{"human_like", "duration_ms", "steps", "jitter_px", "settle_ms", "interval_ms"} {
				if value, ok := input[key]; ok {
					doubleInput[key] = value
				}
			}
			if _, err := DesktopDoubleClickTool(ctx, doubleInput, opts); err != nil {
				return "", err
			}
			result.Action = "double_click"
		default:
			return "", fmt.Errorf("unsupported action for %s target: %s", result.Strategy, action)
		}
	case "window":
		if result.Window == nil {
			return "", fmt.Errorf("window target is missing window details")
		}
		switch action {
		case "auto", "focus":
			if err := focusDesktopTargetWindow(ctx, input, result.Window, opts); err != nil {
				return "", err
			}
			result.Action = "focus"
		case "click", "invoke":
			if err := focusDesktopTargetWindow(ctx, input, result.Window, opts); err != nil {
				return "", err
			}
			clickInput := map[string]any{"x": result.CenterX, "y": result.CenterY}
			for _, key := range []string{"button", "human_like", "duration_ms", "steps", "jitter_px", "settle_ms"} {
				if value, ok := input[key]; ok {
					clickInput[key] = value
				}
			}
			if _, err := DesktopClickTool(ctx, clickInput, opts); err != nil {
				return "", err
			}
			result.Action = "click"
		case "double_click":
			if err := focusDesktopTargetWindow(ctx, input, result.Window, opts); err != nil {
				return "", err
			}
			doubleInput := map[string]any{"x": result.CenterX, "y": result.CenterY}
			for _, key := range []string{"button", "human_like", "duration_ms", "steps", "jitter_px", "settle_ms", "interval_ms"} {
				if value, ok := input[key]; ok {
					doubleInput[key] = value
				}
			}
			if _, err := DesktopDoubleClickTool(ctx, doubleInput, opts); err != nil {
				return "", err
			}
			result.Action = "double_click"
		default:
			return "", fmt.Errorf("unsupported action for window target: %s", action)
		}
	default:
		return "", fmt.Errorf("unsupported target strategy: %s", result.Strategy)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopSetTargetValueTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_set_target_value", opts, false); err != nil {
		return "", err
	}
	value := stringValue(input["value"])
	if value == "" {
		return "", fmt.Errorf("value is required")
	}
	result, err := resolveDesktopTarget(ctx, input, opts)
	if err != nil {
		return "", err
	}
	if !result.Found {
		return "", desktopTargetNotFoundError(result)
	}
	appendValue := boolValue(input["append"])
	submit := boolValue(input["submit"])
	switch result.Strategy {
	case "ui":
		raw, err := DesktopSetValueUITool(ctx, input, opts)
		if err != nil {
			return "", err
		}
		var uiResult desktopAutomationActionResult
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &uiResult); err == nil {
			result.Action = uiResult.Action
			if uiResult.Element != nil {
				result.Element = uiResult.Element
				applyDesktopElementBounds(&result, *uiResult.Element)
			}
		} else {
			result.Action = "value"
			result.Meta = mergeMeta(result.Meta, map[string]any{"set_value_result": raw})
		}
	case "text", "image", "window":
		if err := focusDesktopTargetWindow(ctx, input, result.Window, opts); err != nil {
			return "", err
		}
		if _, err := DesktopClickTool(ctx, map[string]any{"x": result.CenterX, "y": result.CenterY}, opts); err != nil {
			return "", err
		}
		if !appendValue {
			if _, err := DesktopHotkeyTool(ctx, map[string]any{"keys": []any{"ctrl", "a"}}, opts); err != nil {
				return "", err
			}
		}
		if _, err := DesktopTypeTool(ctx, map[string]any{"text": value}, opts); err != nil {
			return "", err
		}
		if submit {
			if _, err := DesktopHotkeyTool(ctx, map[string]any{"keys": []any{"enter"}}, opts); err != nil {
				return "", err
			}
		}
		result.Action = "type"
		result.Meta = mergeMeta(result.Meta, map[string]any{"fallback": "click_and_type"})
	default:
		return "", fmt.Errorf("unsupported target strategy: %s", result.Strategy)
	}
	result.Meta = mergeMeta(result.Meta, map[string]any{
		"value":  value,
		"append": appendValue,
		"submit": submit,
	})
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func resolveDesktopTarget(ctx context.Context, input map[string]any, opts BuiltinOptions) (desktopTargetResolution, error) {
	strategies, err := resolveDesktopTargetStrategies(input)
	if err != nil {
		return desktopTargetResolution{}, err
	}
	result := desktopTargetResolution{
		Meta: map[string]any{
			"strategies": strategies,
		},
	}
	window, windowErr := resolveDesktopTargetWindow(ctx, input)
	if window != nil {
		result.Window = window
	}
	var failures []string
	for _, strategy := range strategies {
		switch strategy {
		case "ui":
			element, err := findDesktopTargetAutomationElement(ctx, input, opts)
			if err != nil {
				failures = append(failures, "ui: "+err.Error())
				continue
			}
			result.Found = true
			result.Strategy = "ui"
			result.Element = element
			applyDesktopElementBounds(&result, *element)
			result.Meta = mergeMeta(result.Meta, map[string]any{"matched_strategy": "ui"})
			return result, nil
		case "text":
			textResult, err := runDesktopTargetTextMatch(ctx, input, opts, window)
			if err != nil {
				failures = append(failures, "text: "+err.Error())
				continue
			}
			if !textResult.Found {
				failures = append(failures, "text: text not found")
				continue
			}
			result.Found = true
			result.Strategy = "text"
			result.Text = &textResult
			applyDesktopTextBounds(&result, textResult)
			result.Meta = mergeMeta(result.Meta, map[string]any{"matched_strategy": "text"})
			return result, nil
		case "image":
			imageResult, err := runDesktopTargetImageMatch(ctx, input, opts, window)
			if err != nil {
				failures = append(failures, "image: "+err.Error())
				continue
			}
			if !imageResult.Found {
				failures = append(failures, fmt.Sprintf("image: best score %.4f below threshold %.4f", imageResult.Score, imageResult.Threshold))
				continue
			}
			result.Found = true
			result.Strategy = "image"
			result.Image = &imageResult
			applyDesktopImageBounds(&result, imageResult)
			result.Meta = mergeMeta(result.Meta, map[string]any{"matched_strategy": "image"})
			return result, nil
		case "window":
			if windowErr != nil {
				failures = append(failures, "window: "+windowErr.Error())
				continue
			}
			if window == nil {
				failures = append(failures, "window: window not found")
				continue
			}
			result.Found = true
			result.Strategy = "window"
			applyDesktopWindowBounds(&result, *window)
			result.Meta = mergeMeta(result.Meta, map[string]any{"matched_strategy": "window"})
			return result, nil
		}
	}
	if len(failures) > 0 {
		result.Meta = mergeMeta(result.Meta, map[string]any{"last_error": strings.Join(failures, "; ")})
	}
	return result, nil
}

func resolveDesktopTargetStrategies(input map[string]any) ([]string, error) {
	if raw, ok := input["strategies"]; ok {
		items, err := normalizeDesktopTargetStrategies(raw)
		if err != nil {
			return nil, err
		}
		if len(items) == 1 && items[0] == "auto" {
			return inferDesktopTargetStrategies(input)
		}
		for _, item := range items {
			if item == "auto" {
				return nil, fmt.Errorf("auto cannot be combined with other strategies")
			}
		}
		return dedupeDesktopTargetStrategies(items), nil
	}
	if raw := strings.TrimSpace(strings.ToLower(stringValue(input["strategy"]))); raw != "" {
		if raw == "auto" {
			return inferDesktopTargetStrategies(input)
		}
		return normalizeDesktopTargetStrategies(raw)
	}
	return inferDesktopTargetStrategies(input)
}

func inferDesktopTargetStrategies(input map[string]any) ([]string, error) {
	strategies := make([]string, 0, 4)
	if desktopTargetUISelectionProvided(input) && desktopTargetWindowSelectionProvided(input) {
		strategies = append(strategies, "ui")
	}
	if strings.TrimSpace(stringValue(input["text"])) != "" {
		strategies = append(strategies, "text")
	}
	if strings.TrimSpace(stringValue(input["template_path"])) != "" {
		strategies = append(strategies, "image")
	}
	if desktopTargetWindowSelectionProvided(input) {
		strategies = append(strategies, "window")
	}
	strategies = dedupeDesktopTargetStrategies(strategies)
	if len(strategies) == 0 {
		return nil, fmt.Errorf("a target selector is required: provide window, ui, text, or template_path criteria")
	}
	return strategies, nil
}

func normalizeDesktopTargetStrategies(value any) ([]string, error) {
	rawItems := make([]string, 0)
	switch v := value.(type) {
	case string:
		for _, item := range strings.Split(v, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				rawItems = append(rawItems, item)
			}
		}
	case []string:
		rawItems = append(rawItems, v...)
	case []any:
		for _, item := range v {
			rawItems = append(rawItems, fmt.Sprint(item))
		}
	default:
		return nil, fmt.Errorf("strategies must be a string or string array")
	}
	if len(rawItems) == 0 {
		return nil, fmt.Errorf("at least one strategy is required")
	}
	items := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		normalized := strings.TrimSpace(strings.ToLower(item))
		switch normalized {
		case "auto", "window", "ui", "text", "image":
			items = append(items, normalized)
		default:
			return nil, fmt.Errorf("unsupported target strategy: %s", item)
		}
	}
	return dedupeDesktopTargetStrategies(items), nil
}

func dedupeDesktopTargetStrategies(items []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func desktopTargetWindowSelectionProvided(input map[string]any) bool {
	selector, _ := resolveDesktopAutomationSelector(input, false)
	return desktopWindowSelectionProvided(selector)
}

func desktopTargetUISelectionProvided(input map[string]any) bool {
	selector, _ := resolveDesktopAutomationSelector(input, false)
	return desktopAutomationFilterProvided(selector)
}

func desktopAutomationFilterProvided(selector desktopAutomationSelector) bool {
	return selector.Name != "" || selector.AutomationID != "" || selector.ClassName != "" || selector.ControlType != ""
}

func resolveDesktopTargetWindow(ctx context.Context, input map[string]any) (*desktopWindowInfo, error) {
	if !desktopTargetWindowSelectionProvided(input) {
		return nil, nil
	}
	windows, err := runDesktopWindowQuery(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, nil
	}
	window := windows[0]
	return &window, nil
}

func findDesktopTargetAutomationElement(ctx context.Context, input map[string]any, opts BuiltinOptions) (*desktopAutomationElement, error) {
	selector, err := resolveDesktopAutomationSelector(input, true)
	if err != nil {
		return nil, err
	}
	if !desktopAutomationFilterProvided(selector) {
		return nil, fmt.Errorf("ui target requires name, automation_id, class_name, or control_type")
	}
	inspectInput := cloneInputMap(input)
	inspectInput["max_elements"] = selector.Index
	raw, err := DesktopInspectUITool(ctx, inspectInput, opts)
	if err != nil {
		return nil, err
	}
	elements := []desktopAutomationElement{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &elements); err != nil {
		return nil, err
	}
	if len(elements) < selector.Index {
		return nil, fmt.Errorf("automation element not found")
	}
	element := elements[selector.Index-1]
	return &element, nil
}

func runDesktopTargetTextMatch(ctx context.Context, input map[string]any, opts BuiltinOptions, window *desktopWindowInfo) (ocrTextMatchResult, error) {
	source, err := resolveDesktopTargetVisionSource(ctx, input, opts, window)
	if err != nil {
		return ocrTextMatchResult{}, err
	}
	defer source.Cleanup()
	matchInput := cloneInputMap(input)
	matchInput["path"] = source.Path
	result, err := runDesktopTextMatch(ctx, matchInput, opts)
	if err != nil {
		return ocrTextMatchResult{}, err
	}
	applyTextMatchOffset(&result, source.OffsetX, source.OffsetY)
	return result, nil
}

func runDesktopTargetImageMatch(ctx context.Context, input map[string]any, opts BuiltinOptions, window *desktopWindowInfo) (imageMatchResult, error) {
	source, err := resolveDesktopTargetVisionSource(ctx, input, opts, window)
	if err != nil {
		return imageMatchResult{}, err
	}
	defer source.Cleanup()
	matchInput := cloneInputMap(input)
	matchInput["path"] = source.Path
	result, err := runDesktopImageMatch(ctx, matchInput, opts)
	if err != nil {
		return imageMatchResult{}, err
	}
	applyImageMatchOffset(&result, source.OffsetX, source.OffsetY)
	return result, nil
}

func resolveDesktopTargetVisionSource(ctx context.Context, input map[string]any, opts BuiltinOptions, window *desktopWindowInfo) (desktopVisionSource, error) {
	path, cleanup, err := resolveDesktopVisionSource(ctx, input, opts)
	if err != nil {
		return desktopVisionSource{}, err
	}
	source := desktopVisionSource{
		Path:    path,
		Cleanup: cleanup,
	}
	if !shouldCropDesktopTargetToWindow(input, window) {
		return source, nil
	}
	croppedPath, offsetX, offsetY, cropCleanup, err := cropDesktopTargetImageToWindow(path, *window)
	if err != nil {
		cleanup()
		return desktopVisionSource{}, err
	}
	source.Path = croppedPath
	source.OffsetX = offsetX
	source.OffsetY = offsetY
	source.Cleanup = func() {
		cropCleanup()
		cleanup()
	}
	return source, nil
}

func shouldCropDesktopTargetToWindow(input map[string]any, window *desktopWindowInfo) bool {
	if window == nil {
		return false
	}
	if hasDesktopSearchBounds(input) {
		return false
	}
	if raw, ok := input["crop_to_window"]; ok {
		return boolValue(raw)
	}
	return strings.TrimSpace(stringValue(input["path"])) == ""
}

func hasDesktopSearchBounds(input map[string]any) bool {
	_, hasX := numberInput(input["search_x"])
	_, hasY := numberInput(input["search_y"])
	_, hasWidth := numberInput(input["search_width"])
	_, hasHeight := numberInput(input["search_height"])
	return hasX || hasY || hasWidth || hasHeight
}

func cropDesktopTargetImageToWindow(path string, window desktopWindowInfo) (string, int, int, func(), error) {
	img, err := loadImageFile(path)
	if err != nil {
		return "", 0, 0, nil, err
	}
	bounds := normalizeBounds(img.Bounds())
	cropRect := image.Rect(window.X, window.Y, window.X+window.Width, window.Y+window.Height).Intersect(bounds)
	if cropRect.Empty() {
		return "", 0, 0, nil, fmt.Errorf("window bounds are outside the captured desktop image")
	}
	dst := image.NewRGBA(image.Rect(0, 0, cropRect.Dx(), cropRect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, cropRect.Min, draw.Src)
	tempFile, err := os.CreateTemp("", "anyclaw-target-window-*.png")
	if err != nil {
		return "", 0, 0, nil, err
	}
	tempPath := tempFile.Name()
	if err := png.Encode(tempFile, dst); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return "", 0, 0, nil, err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", 0, 0, nil, err
	}
	return tempPath, cropRect.Min.X, cropRect.Min.Y, func() { _ = os.Remove(tempPath) }, nil
}

func focusDesktopTargetWindow(ctx context.Context, input map[string]any, window *desktopWindowInfo, opts BuiltinOptions) error {
	if window == nil {
		return nil
	}
	focusInput := map[string]any{}
	if window.Handle > 0 {
		focusInput["handle"] = window.Handle
	}
	if title := firstNonEmpty(window.Title, stringValue(input["title"])); title != "" {
		focusInput["title"] = title
	}
	if processName := firstNonEmpty(window.ProcessName, stringValue(input["process_name"])); processName != "" {
		focusInput["process_name"] = processName
	}
	if match := stringValue(input["match"]); match != "" {
		focusInput["match"] = match
	}
	_, err := DesktopFocusWindowTool(ctx, focusInput, opts)
	return err
}

func normalizeDesktopTargetAction(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "auto", nil
	}
	switch value {
	case "auto", "click", "double_click", "focus", "invoke", "select", "expand", "collapse", "toggle":
		return value, nil
	default:
		return "", fmt.Errorf("action must be auto, click, double_click, focus, invoke, select, expand, collapse, or toggle")
	}
}

func applyDesktopWindowBounds(result *desktopTargetResolution, window desktopWindowInfo) {
	result.X = window.X
	result.Y = window.Y
	result.Width = window.Width
	result.Height = window.Height
	result.CenterX = window.CenterX
	result.CenterY = window.CenterY
}

func applyDesktopElementBounds(result *desktopTargetResolution, element desktopAutomationElement) {
	result.X = element.X
	result.Y = element.Y
	result.Width = element.Width
	result.Height = element.Height
	result.CenterX = element.CenterX
	result.CenterY = element.CenterY
}

func applyDesktopTextBounds(result *desktopTargetResolution, match ocrTextMatchResult) {
	result.X = match.X
	result.Y = match.Y
	result.Width = match.Width
	result.Height = match.Height
	result.CenterX = match.CenterX
	result.CenterY = match.CenterY
}

func applyDesktopImageBounds(result *desktopTargetResolution, match imageMatchResult) {
	result.X = match.X
	result.Y = match.Y
	result.Width = match.Width
	result.Height = match.Height
	result.CenterX = match.CenterX
	result.CenterY = match.CenterY
}

func applyTextMatchOffset(result *ocrTextMatchResult, offsetX int, offsetY int) {
	if result == nil || (offsetX == 0 && offsetY == 0) {
		return
	}
	result.X += offsetX
	result.Y += offsetY
	result.CenterX += offsetX
	result.CenterY += offsetY
}

func applyImageMatchOffset(result *imageMatchResult, offsetX int, offsetY int) {
	if result == nil || (offsetX == 0 && offsetY == 0) {
		return
	}
	result.X += offsetX
	result.Y += offsetY
	result.CenterX += offsetX
	result.CenterY += offsetY
}

func desktopTargetNotFoundError(result desktopTargetResolution) error {
	if result.Meta != nil {
		if lastErr, ok := result.Meta["last_error"].(string); ok && strings.TrimSpace(lastErr) != "" {
			return fmt.Errorf("target not found: %s", lastErr)
		}
	}
	return fmt.Errorf("target not found")
}

func cloneInputMap(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
