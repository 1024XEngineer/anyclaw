package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	computerProtocolVersion        = "anyclaw.computer.codex.v1"
	defaultComputerCoordinateSpace = "normalized_0_1000"
)

type ComputerController interface {
	Observe(ctx context.Context, req ComputerObserveRequest) (*ComputerObservation, error)
	Act(ctx context.Context, req ComputerActionRequest) (*ComputerObservation, error)
}

type ComputerOptions struct {
	Enabled               bool
	Backend               string
	CoordinateSpace       string
	MaxActionsPerTurn     int
	ObserveAfterAction    bool
	IncludeWindowsDefault bool
	RedactTextInAudit     bool
	AllowedApps           []string
	AllowedDomains        []string
}

type ComputerObserveRequest struct {
	Path                    string
	IncludeScreenshotBase64 bool
	IncludeWindows          bool
}

type ComputerActionRequest struct {
	Actions                 []ComputerAction
	Path                    string
	IncludeScreenshotBase64 bool
	IncludeWindows          bool
	ObserveAfterAction      bool
}

type ComputerAction struct {
	Type              string         `json:"type"`
	X                 int            `json:"x,omitempty"`
	Y                 int            `json:"y,omitempty"`
	DestinationX      int            `json:"destination_x,omitempty"`
	DestinationY      int            `json:"destination_y,omitempty"`
	CoordinateSpace   string         `json:"coordinate_space,omitempty"`
	Text              string         `json:"text,omitempty"`
	Keys              []string       `json:"keys,omitempty"`
	Direction         string         `json:"direction,omitempty"`
	Delta             int            `json:"delta,omitempty"`
	Clicks            int            `json:"clicks,omitempty"`
	Button            string         `json:"button,omitempty"`
	WaitMS            int            `json:"wait_ms,omitempty"`
	URL               string         `json:"url,omitempty"`
	Query             string         `json:"query,omitempty"`
	Submit            bool           `json:"submit,omitempty"`
	ClearBeforeTyping bool           `json:"clear_before_typing,omitempty"`
	HumanLike         bool           `json:"human_like,omitempty"`
	DurationMS        int            `json:"duration_ms,omitempty"`
	Steps             int            `json:"steps,omitempty"`
	JitterPX          int            `json:"jitter_px,omitempty"`
	SettleMS          int            `json:"settle_ms,omitempty"`
	IntervalMS        int            `json:"interval_ms,omitempty"`
	Meta              map[string]any `json:"meta,omitempty"`
	HasX              bool           `json:"has_x,omitempty"`
	HasY              bool           `json:"has_y,omitempty"`
	HasDestinationX   bool           `json:"has_destination_x,omitempty"`
	HasDestinationY   bool           `json:"has_destination_y,omitempty"`
	hasX              bool
	hasY              bool
	hasDestinationX   bool
	hasDestinationY   bool
}

type computerScreenMetrics struct {
	OriginX         int    `json:"origin_x"`
	OriginY         int    `json:"origin_y"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	CursorX         int    `json:"cursor_x,omitempty"`
	CursorY         int    `json:"cursor_y,omitempty"`
	CoordinateSpace string `json:"coordinate_space"`
}

type ComputerObservation struct {
	Protocol         string                   `json:"protocol"`
	Environment      string                   `json:"environment"`
	Backend          string                   `json:"backend,omitempty"`
	Timestamp        string                   `json:"timestamp"`
	URL              string                   `json:"url,omitempty"`
	ScreenshotPath   string                   `json:"screenshot_path,omitempty"`
	ScreenshotMIME   string                   `json:"screenshot_mime,omitempty"`
	ScreenshotBase64 string                   `json:"screenshot_base64,omitempty"`
	Screen           computerScreenMetrics    `json:"screen"`
	ActiveWindow     *desktopWindowInfo       `json:"active_window,omitempty"`
	Windows          []desktopWindowInfo      `json:"windows,omitempty"`
	Action           *computerActionSnapshot  `json:"action,omitempty"`
	Actions          []computerActionSnapshot `json:"actions,omitempty"`
	Meta             map[string]any           `json:"meta,omitempty"`
}

type computerActionSnapshot struct {
	Name        string         `json:"name"`
	Type        string         `json:"type,omitempty"`
	CoordinateX int            `json:"coordinate_x,omitempty"`
	CoordinateY int            `json:"coordinate_y,omitempty"`
	Output      string         `json:"output,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

type codexLocalComputerController struct {
	opts BuiltinOptions
}

type unsupportedComputerController struct {
	backend string
}

func ComputerObserveTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if !computerEnabled(opts.Computer) {
		return "", fmt.Errorf("computer control is disabled")
	}
	if err := ensureDesktopAllowed(ctx, "computer_observe", opts, true); err != nil {
		return "", err
	}
	req := parseComputerObserveRequest(input, opts.Computer)
	state, err := computerControllerFromOptions(opts).Observe(ctx, req)
	if err != nil {
		return "", err
	}
	return marshalCompactJSON(state)
}

func ComputerActionTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if !computerEnabled(opts.Computer) {
		return "", fmt.Errorf("computer control is disabled")
	}
	req, err := parseComputerActionRequest(input, opts.Computer)
	if err != nil {
		return "", err
	}
	if err := ensureDesktopAllowed(ctx, "computer_action", opts, false); err != nil {
		return "", err
	}
	state, err := computerControllerFromOptions(opts).Act(ctx, req)
	if err != nil {
		return "", err
	}
	return marshalCompactJSON(state)
}

func computerControllerFromOptions(opts BuiltinOptions) ComputerController {
	if opts.ComputerController != nil {
		return opts.ComputerController
	}
	return NewComputerController(opts)
}

func NewComputerController(opts BuiltinOptions) ComputerController {
	backend := normalizeComputerBackend(opts.Computer.Backend)
	switch backend {
	case "codex_local":
		localOpts := opts
		localOpts.Computer.Backend = "codex_local"
		return &codexLocalComputerController{opts: localOpts}
	default:
		return &unsupportedComputerController{backend: backend}
	}
}

func (c *unsupportedComputerController) Observe(ctx context.Context, req ComputerObserveRequest) (*ComputerObservation, error) {
	return nil, c.err()
}

func (c *unsupportedComputerController) Act(ctx context.Context, req ComputerActionRequest) (*ComputerObservation, error) {
	return nil, c.err()
}

func (c *unsupportedComputerController) err() error {
	backend := strings.TrimSpace(c.backend)
	if backend == "" {
		backend = "unknown"
	}
	return fmt.Errorf("unsupported computer.backend %q; supported backends: codex_local", backend)
}

func (c *codexLocalComputerController) Observe(ctx context.Context, req ComputerObserveRequest) (*ComputerObservation, error) {
	state, err := observeComputerState(ctx, c.opts, req, nil)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (c *codexLocalComputerController) Act(ctx context.Context, req ComputerActionRequest) (*ComputerObservation, error) {
	maxActions := c.opts.Computer.MaxActionsPerTurn
	if maxActions <= 0 {
		maxActions = 8
	}
	if len(req.Actions) == 0 {
		return nil, fmt.Errorf("action or actions is required")
	}
	if len(req.Actions) > maxActions {
		return nil, fmt.Errorf("too many computer actions: got %d, max %d", len(req.Actions), maxActions)
	}

	snapshots := make([]computerActionSnapshot, 0, len(req.Actions))
	for _, action := range req.Actions {
		snapshot, err := c.executeAction(ctx, action)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, *snapshot)
	}

	if !req.ObserveAfterAction {
		state := ComputerObservation{
			Protocol:    computerProtocolVersion,
			Environment: "desktop",
			Backend:     normalizeComputerBackend(c.opts.Computer.Backend),
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Actions:     snapshots,
		}
		if len(snapshots) == 1 {
			state.Action = &snapshots[0]
		}
		return &state, nil
	}

	state, err := observeComputerState(ctx, c.opts, ComputerObserveRequest{
		Path:                    req.Path,
		IncludeScreenshotBase64: req.IncludeScreenshotBase64,
		IncludeWindows:          req.IncludeWindows,
	}, snapshots)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (c *codexLocalComputerController) executeAction(ctx context.Context, action ComputerAction) (*computerActionSnapshot, error) {
	action.Type = normalizeCodexComputerAction(action.Type)
	if action.Type == "" {
		return nil, fmt.Errorf("action type is required")
	}
	snapshot := &computerActionSnapshot{Name: action.Type, Type: action.Type}
	run := func(args map[string]any, fn func(context.Context, map[string]any, BuiltinOptions) (string, error)) error {
		output, err := fn(ctx, args, c.opts)
		if err != nil {
			return err
		}
		snapshot.Output = strings.TrimSpace(output)
		return nil
	}

	switch action.Type {
	case "open_url":
		if err := c.ensureActionAllowed(ctx, action); err != nil {
			return nil, err
		}
		target := strings.TrimSpace(firstNonEmpty(action.URL, action.Query))
		if target == "" {
			return nil, fmt.Errorf("url is required")
		}
		if err := run(map[string]any{"target": target, "kind": "url"}, DesktopOpenTool); err != nil {
			return nil, err
		}
		snapshot.Meta = map[string]any{"url": target}
	case "search":
		if err := c.ensureActionAllowed(ctx, action); err != nil {
			return nil, err
		}
		target := "https://www.google.com"
		if strings.TrimSpace(action.Query) != "" {
			target = "https://www.google.com/search?q=" + url.QueryEscape(strings.TrimSpace(action.Query))
		}
		if err := run(map[string]any{"target": target, "kind": "url"}, DesktopOpenTool); err != nil {
			return nil, err
		}
		snapshot.Meta = map[string]any{"url": target}
	case "click", "double_click", "move":
		if err := c.ensureActionAllowed(ctx, action); err != nil {
			return nil, err
		}
		x, y, err := c.actionPoint(ctx, action)
		if err != nil {
			return nil, err
		}
		snapshot.CoordinateX = x
		snapshot.CoordinateY = y
		args := computerMouseArgs(action, x, y)
		switch action.Type {
		case "click":
			if err := run(args, DesktopClickTool); err != nil {
				return nil, err
			}
		case "double_click":
			if err := run(args, DesktopDoubleClickTool); err != nil {
				return nil, err
			}
		case "move":
			if err := run(args, DesktopMoveTool); err != nil {
				return nil, err
			}
		}
	case "type":
		if err := c.ensureActionAllowed(ctx, action); err != nil {
			return nil, err
		}
		if action.Text == "" {
			return nil, fmt.Errorf("text is required")
		}
		output, err := DesktopPasteTool(ctx, map[string]any{
			"text":   action.Text,
			"submit": action.Submit,
		}, c.opts)
		if err != nil {
			return nil, err
		}
		snapshot.Output = strings.TrimSpace(output)
		snapshot.Meta = map[string]any{
			"text_length": len([]rune(action.Text)),
			"submit":      action.Submit,
		}
	case "keypress":
		if err := c.ensureActionAllowed(ctx, action); err != nil {
			return nil, err
		}
		if len(action.Keys) == 0 {
			return nil, fmt.Errorf("keys is required")
		}
		keys := make([]any, 0, len(action.Keys))
		for _, key := range action.Keys {
			keys = append(keys, key)
		}
		if err := run(map[string]any{"keys": keys}, DesktopHotkeyTool); err != nil {
			return nil, err
		}
		snapshot.Meta = map[string]any{"keys": action.Keys}
	case "scroll":
		if err := c.ensureActionAllowed(ctx, action); err != nil {
			return nil, err
		}
		args := map[string]any{}
		if action.Direction != "" {
			args["direction"] = action.Direction
		}
		if action.Delta != 0 {
			args["delta"] = action.Delta
		}
		if action.Clicks != 0 {
			args["clicks"] = action.Clicks
		}
		if computerPointProvided(action) {
			x, y, err := c.actionPoint(ctx, action)
			if err != nil {
				return nil, err
			}
			args["x"] = x
			args["y"] = y
			snapshot.CoordinateX = x
			snapshot.CoordinateY = y
		}
		if err := run(args, DesktopScrollTool); err != nil {
			return nil, err
		}
	case "drag":
		if err := c.ensureActionAllowed(ctx, action); err != nil {
			return nil, err
		}
		x1, y1, err := c.actionPoint(ctx, action)
		if err != nil {
			return nil, err
		}
		x2, y2, err := c.destinationPoint(ctx, action)
		if err != nil {
			return nil, err
		}
		args := map[string]any{"x1": x1, "y1": y1, "x2": x2, "y2": y2}
		if action.Button != "" {
			args["button"] = action.Button
		}
		if action.DurationMS > 0 {
			args["duration_ms"] = action.DurationMS
		}
		if action.Steps > 0 {
			args["steps"] = action.Steps
		}
		if err := run(args, DesktopDragTool); err != nil {
			return nil, err
		}
		snapshot.CoordinateX = x1
		snapshot.CoordinateY = y1
		snapshot.Meta = map[string]any{"destination_x": x2, "destination_y": y2}
	case "wait":
		waitMS := action.WaitMS
		if waitMS <= 0 {
			waitMS = 5000
		}
		if err := run(map[string]any{"wait_ms": waitMS}, DesktopWaitTool); err != nil {
			return nil, err
		}
		snapshot.Meta = map[string]any{"wait_ms": waitMS}
	default:
		return nil, fmt.Errorf("unsupported computer action: %s", action.Type)
	}

	return snapshot, nil
}

func parseComputerObserveRequest(input map[string]any, opts ComputerOptions) ComputerObserveRequest {
	return ComputerObserveRequest{
		Path:                    strings.TrimSpace(stringValue(input["path"])),
		IncludeScreenshotBase64: boolValue(input["include_screenshot_base64"]),
		IncludeWindows:          computerBoolDefault(input, "include_windows", opts.IncludeWindowsDefault),
	}
}

func parseComputerActionRequest(input map[string]any, opts ComputerOptions) (ComputerActionRequest, error) {
	actions, err := parseComputerActions(input, opts)
	if err != nil {
		return ComputerActionRequest{}, err
	}
	return ComputerActionRequest{
		Actions:                 actions,
		Path:                    strings.TrimSpace(stringValue(input["path"])),
		IncludeScreenshotBase64: boolValue(input["include_screenshot_base64"]),
		IncludeWindows:          computerBoolDefault(input, "include_windows", opts.IncludeWindowsDefault),
		ObserveAfterAction:      computerBoolDefault(input, "observe_after_action", defaultComputerObserveAfterAction(opts)),
	}, nil
}

func parseComputerActions(input map[string]any, opts ComputerOptions) ([]ComputerAction, error) {
	if rawActions, ok := input["actions"]; ok {
		actions, err := decodeComputerActions(rawActions)
		if err != nil {
			return nil, err
		}
		if len(actions) == 0 {
			return nil, fmt.Errorf("actions must not be empty")
		}
		return normalizeComputerActions(actions, opts), nil
	}

	actionName := normalizeComputerAction(stringValue(input["action"]))
	if actionName == "" {
		return nil, fmt.Errorf("action is required")
	}
	return legacyComputerActionToCodexActions(actionName, input, opts)
}

func decodeComputerActions(value any) ([]ComputerAction, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to encode actions: %w", err)
	}
	var rawActions []map[string]any
	if err := json.Unmarshal(data, &rawActions); err != nil {
		return nil, fmt.Errorf("actions must be an array of computer actions: %w", err)
	}
	actions := make([]ComputerAction, 0, len(rawActions))
	for _, raw := range rawActions {
		actions = append(actions, computerActionFromMap(raw))
	}
	return actions, nil
}

func normalizeComputerActions(actions []ComputerAction, opts ComputerOptions) []ComputerAction {
	out := make([]ComputerAction, 0, len(actions))
	defaultSpace := defaultComputerActionCoordinateSpace(opts)
	for _, action := range actions {
		action.Type = normalizeCodexComputerAction(firstNonEmpty(action.Type, stringValue(action.Meta["action"])))
		if action.CoordinateSpace == "" {
			action.CoordinateSpace = defaultSpace
		}
		out = append(out, action)
	}
	return out
}

func computerActionFromMap(raw map[string]any) ComputerAction {
	action := ComputerAction{
		Type:              firstNonEmpty(stringValue(raw["type"]), stringValue(raw["action"])),
		CoordinateSpace:   stringValue(raw["coordinate_space"]),
		Text:              stringValue(raw["text"]),
		Keys:              nil,
		Direction:         stringValue(raw["direction"]),
		Button:            stringValue(raw["button"]),
		URL:               firstNonEmpty(stringValue(raw["url"]), stringValue(raw["target"])),
		Query:             stringValue(raw["query"]),
		Submit:            boolValue(raw["submit"]) || boolValue(raw["press_enter"]),
		ClearBeforeTyping: computerBoolDefault(raw, "clear_before_typing", false),
		HumanLike:         boolValue(raw["human_like"]),
		DurationMS:        intNumberWithDefault(raw["duration_ms"], 0),
		Steps:             intNumberWithDefault(raw["steps"], 0),
		JitterPX:          intNumberWithDefault(raw["jitter_px"], 0),
		SettleMS:          intNumberWithDefault(raw["settle_ms"], 0),
		IntervalMS:        intNumberWithDefault(raw["interval_ms"], 0),
		WaitMS:            intNumberWithDefault(raw["wait_ms"], 0),
		Clicks:            intNumberWithDefault(raw["clicks"], 0),
		Delta:             intNumberWithDefault(raw["delta"], 0),
	}
	if x, ok := numberInput(raw["x"]); ok {
		action.X = x
		action.hasX = true
		action.HasX = true
	}
	if y, ok := numberInput(raw["y"]); ok {
		action.Y = y
		action.hasY = true
		action.HasY = true
	}
	if x, ok := numberInput(raw["destination_x"]); ok {
		action.DestinationX = x
		action.hasDestinationX = true
		action.HasDestinationX = true
	}
	if y, ok := numberInput(raw["destination_y"]); ok {
		action.DestinationY = y
		action.hasDestinationY = true
		action.HasDestinationY = true
	}
	if keys, err := computerActionStringKeys(raw["keys"]); err == nil {
		action.Keys = keys
	}
	if meta, ok := raw["meta"].(map[string]any); ok {
		action.Meta = meta
	}
	return action
}

func legacyComputerActionToCodexActions(action string, input map[string]any, opts ComputerOptions) ([]ComputerAction, error) {
	base := ComputerAction{
		Type:              action,
		CoordinateSpace:   stringValue(input["coordinate_space"]),
		Text:              stringValue(input["text"]),
		Direction:         stringValue(input["direction"]),
		Button:            stringValue(input["button"]),
		URL:               firstNonEmpty(stringValue(input["url"]), stringValue(input["target"])),
		Query:             stringValue(input["query"]),
		Submit:            boolValue(input["press_enter"]) || boolValue(input["submit"]),
		ClearBeforeTyping: computerBoolDefault(input, "clear_before_typing", true),
		HumanLike:         boolValue(input["human_like"]),
		DurationMS:        intNumberWithDefault(input["duration_ms"], 0),
		Steps:             intNumberWithDefault(input["steps"], 0),
		JitterPX:          intNumberWithDefault(input["jitter_px"], 0),
		SettleMS:          intNumberWithDefault(input["settle_ms"], 0),
		IntervalMS:        intNumberWithDefault(input["interval_ms"], 0),
		WaitMS:            intNumberWithDefault(input["wait_ms"], 0),
		Clicks:            intNumberWithDefault(input["clicks"], 0),
		Delta:             intNumberWithDefault(input["delta"], 0),
	}
	if x, ok := numberInput(input["x"]); ok {
		base.X = x
		base.hasX = true
		base.HasX = true
	}
	if y, ok := numberInput(input["y"]); ok {
		base.Y = y
		base.hasY = true
		base.HasY = true
	}
	if x, ok := numberInput(input["destination_x"]); ok {
		base.DestinationX = x
		base.hasDestinationX = true
		base.HasDestinationX = true
	}
	if y, ok := numberInput(input["destination_y"]); ok {
		base.DestinationY = y
		base.hasDestinationY = true
		base.HasDestinationY = true
	}
	if keys, err := computerActionStringKeys(input["keys"]); err == nil {
		base.Keys = keys
	}

	switch action {
	case "open_web_browser":
		if base.URL == "" {
			base.URL = "https://www.google.com"
		}
		base.Type = "open_url"
		return []ComputerAction{base}, nil
	case "navigate":
		if strings.TrimSpace(base.URL) == "" {
			return nil, fmt.Errorf("url is required")
		}
		base.Type = "open_url"
		return []ComputerAction{base}, nil
	case "search":
		target := "https://www.google.com"
		if strings.TrimSpace(base.Query) != "" {
			target = "https://www.google.com/search?q=" + url.QueryEscape(strings.TrimSpace(base.Query))
		}
		base.Type = "open_url"
		base.URL = target
		return []ComputerAction{base}, nil
	case "click_at":
		base.Type = "click"
	case "double_click_at":
		base.Type = "double_click"
	case "hover_at":
		base.Type = "move"
	case "type_text_at":
		if base.Text == "" {
			return nil, fmt.Errorf("text is required")
		}
		click := base
		click.Type = "click"
		click.Text = ""
		actions := []ComputerAction{click}
		if base.ClearBeforeTyping {
			actions = append(actions, ComputerAction{
				Type: "keypress",
				Keys: []string{"ctrl", "a"},
			})
		}
		typeAction := base
		typeAction.Type = "type"
		actions = append(actions, typeAction)
		return actions, nil
	case "scroll_document", "scroll_at":
		base.Type = "scroll"
		if base.Direction == "" {
			base.Direction = "down"
		}
		if base.Clicks <= 0 && base.Delta == 0 {
			base.Clicks = 5
		}
	case "key_combination":
		if len(base.Keys) == 0 {
			return nil, fmt.Errorf("keys is required")
		}
		base.Type = "keypress"
	case "drag_and_drop":
		base.Type = "drag"
	case "go_back":
		base.Type = "keypress"
		base.Keys = []string{"alt", "left"}
	case "go_forward":
		base.Type = "keypress"
		base.Keys = []string{"alt", "right"}
	case "wait", "wait_5_seconds":
		base.Type = "wait"
		if action == "wait_5_seconds" || base.WaitMS <= 0 {
			base.WaitMS = 5000
		}
	default:
		base.Type = normalizeCodexComputerAction(action)
	}
	if base.CoordinateSpace == "" {
		base.CoordinateSpace = defaultComputerActionCoordinateSpace(opts)
	}
	return []ComputerAction{base}, nil
}

func observeComputerState(ctx context.Context, opts BuiltinOptions, req ComputerObserveRequest, actions []computerActionSnapshot) (ComputerObservation, error) {
	screen, screenErr := getComputerScreenMetrics(ctx, opts.Computer)
	path := computerScreenshotPath(req.Path)
	if _, err := DesktopScreenshotTool(ctx, map[string]any{"path": path}, opts); err != nil {
		return ComputerObservation{}, err
	}
	resolved := resolvePath(path, opts.WorkingDir)
	width, height := computerScreenshotDimensions(resolved)
	if screenErr != nil {
		screen = computerScreenMetrics{Width: width, Height: height}
	} else if screen.Width <= 0 || screen.Height <= 0 {
		screen.Width = width
		screen.Height = height
	}
	screen.CoordinateSpace = displayComputerCoordinateSpace(opts.Computer)

	var activeWindow *desktopWindowInfo
	if windows, err := runDesktopWindowQuery(ctx, map[string]any{"active_only": true}); err == nil && len(windows) > 0 {
		window := windows[0]
		activeWindow = &window
	}

	state := ComputerObservation{
		Protocol:       computerProtocolVersion,
		Environment:    "desktop",
		Backend:        normalizeComputerBackend(opts.Computer.Backend),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		ScreenshotPath: resolved,
		ScreenshotMIME: "image/png",
		Screen:         screen,
		ActiveWindow:   activeWindow,
		Actions:        actions,
	}
	if len(actions) == 1 {
		state.Action = &actions[0]
	}
	if req.IncludeWindows {
		windows, err := runDesktopWindowQuery(ctx, map[string]any{})
		if err != nil {
			return ComputerObservation{}, err
		}
		state.Windows = windows
	}
	if req.IncludeScreenshotBase64 {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return ComputerObservation{}, fmt.Errorf("failed to read screenshot: %w", err)
		}
		state.ScreenshotBase64 = base64.StdEncoding.EncodeToString(data)
	}
	if screenErr != nil {
		state.Meta = map[string]any{"screen_metrics_error": screenErr.Error()}
	}
	return state, nil
}

func getComputerScreenMetrics(ctx context.Context, opts ComputerOptions) (computerScreenMetrics, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms;
Add-Type @"
using System;
using System.Runtime.InteropServices;
public struct DesktopPoint {
  public int X;
  public int Y;
}
public static class ComputerNative {
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out DesktopPoint lpPoint);
}
"@;
$bounds = [System.Windows.Forms.SystemInformation]::VirtualScreen;
$point = New-Object DesktopPoint;
[ComputerNative]::GetCursorPos([ref]$point) | Out-Null;
[pscustomobject]@{
  origin_x = [int]$bounds.Left
  origin_y = [int]$bounds.Top
  width = [int]$bounds.Width
  height = [int]$bounds.Height
  cursor_x = [int]$point.X
  cursor_y = [int]$point.Y
} | ConvertTo-Json -Depth 3 -Compress
`
	output, err := runDesktopPowerShell(ctx, script)
	if err != nil {
		return computerScreenMetrics{}, err
	}
	var metrics computerScreenMetrics
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &metrics); err != nil {
		return computerScreenMetrics{}, fmt.Errorf("failed to parse screen metrics: %w", err)
	}
	metrics.CoordinateSpace = displayComputerCoordinateSpace(opts)
	return metrics, nil
}

func (c *codexLocalComputerController) actionPoint(ctx context.Context, action ComputerAction) (int, int, error) {
	if !computerPointProvided(action) {
		return 0, 0, fmt.Errorf("x and y are required")
	}
	if computerActionCoordinateSpace(action, c.opts.Computer) == "absolute" {
		return action.X, action.Y, nil
	}
	metrics, err := getComputerScreenMetrics(ctx, c.opts.Computer)
	if err != nil {
		return 0, 0, err
	}
	return denormalizeComputerCoordinate(action.X, metrics.OriginX, metrics.Width), denormalizeComputerCoordinate(action.Y, metrics.OriginY, metrics.Height), nil
}

func (c *codexLocalComputerController) destinationPoint(ctx context.Context, action ComputerAction) (int, int, error) {
	if !computerDestinationProvided(action) {
		return 0, 0, fmt.Errorf("destination_x and destination_y are required")
	}
	if computerActionCoordinateSpace(action, c.opts.Computer) == "absolute" {
		return action.DestinationX, action.DestinationY, nil
	}
	metrics, err := getComputerScreenMetrics(ctx, c.opts.Computer)
	if err != nil {
		return 0, 0, err
	}
	return denormalizeComputerCoordinate(action.DestinationX, metrics.OriginX, metrics.Width), denormalizeComputerCoordinate(action.DestinationY, metrics.OriginY, metrics.Height), nil
}

func computerPointProvided(action ComputerAction) bool {
	hasX := action.hasX || action.HasX
	hasY := action.hasY || action.HasY
	if hasX != hasY {
		return false
	}
	if hasX && hasY {
		return true
	}
	return action.X != 0 || action.Y != 0
}

func computerDestinationProvided(action ComputerAction) bool {
	hasDestinationX := action.hasDestinationX || action.HasDestinationX
	hasDestinationY := action.hasDestinationY || action.HasDestinationY
	if hasDestinationX != hasDestinationY {
		return false
	}
	if hasDestinationX && hasDestinationY {
		return true
	}
	return action.DestinationX != 0 || action.DestinationY != 0
}

func denormalizeComputerCoordinate(value int, origin int, size int) int {
	if size <= 0 {
		return origin
	}
	if value < 0 {
		value = 0
	}
	if value > 1000 {
		value = 1000
	}
	return origin + (value*(size-1)+500)/1000
}

func computerActionCoordinateSpace(action ComputerAction, opts ComputerOptions) string {
	value := strings.TrimSpace(strings.ToLower(firstNonEmpty(action.CoordinateSpace, defaultComputerActionCoordinateSpace(opts))))
	switch value {
	case "absolute", "screen", "pixels":
		return "absolute"
	default:
		return "normalized"
	}
}

func defaultComputerActionCoordinateSpace(opts ComputerOptions) string {
	value := strings.TrimSpace(strings.ToLower(opts.CoordinateSpace))
	if value == "" {
		return defaultComputerCoordinateSpace
	}
	return value
}

func displayComputerCoordinateSpace(opts ComputerOptions) string {
	value := defaultComputerActionCoordinateSpace(opts)
	switch value {
	case "absolute", "screen", "pixels":
		return "absolute"
	case "normalized":
		return defaultComputerCoordinateSpace
	default:
		return value
	}
}

func computerActionKeys(value any) ([]any, error) {
	keys, err := computerActionStringKeys(value)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, key)
	}
	return out, nil
}

func computerActionStringKeys(value any) ([]string, error) {
	switch v := value.(type) {
	case []any:
		if len(v) == 0 {
			return nil, fmt.Errorf("keys is required")
		}
		out := make([]string, 0, len(v))
		for _, item := range v {
			if key := strings.TrimSpace(fmt.Sprint(item)); key != "" {
				out = append(out, key)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("keys is required")
		}
		return out, nil
	case []string:
		if len(v) == 0 {
			return nil, fmt.Errorf("keys is required")
		}
		return v, nil
	case string:
		items := strings.FieldsFunc(v, func(r rune) bool {
			return r == '+' || r == ',' || r == ' '
		})
		out := make([]string, 0, len(items))
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("keys is required")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("keys must be a string or array")
	}
}

func normalizeComputerAction(action string) string {
	action = strings.TrimSpace(strings.ToLower(action))
	action = strings.ReplaceAll(action, "-", "_")
	switch action {
	case "click", "click_at":
		return "click_at"
	case "double_click", "double_click_at":
		return "double_click_at"
	case "hover", "move", "hover_at":
		return "hover_at"
	case "type", "type_text", "type_text_at":
		return "type_text_at"
	case "scroll", "scroll_document":
		return "scroll_document"
	case "scroll_at":
		return "scroll_at"
	case "wait", "wait_5_seconds":
		return action
	case "back", "go_back":
		return "go_back"
	case "forward", "go_forward":
		return "go_forward"
	case "open_web_browser", "navigate", "search", "key_combination", "drag_and_drop":
		return action
	default:
		return action
	}
}

func normalizeCodexComputerAction(action string) string {
	action = strings.TrimSpace(strings.ToLower(action))
	action = strings.ReplaceAll(action, "-", "_")
	switch action {
	case "click", "left_click":
		return "click"
	case "double_click":
		return "double_click"
	case "move", "hover":
		return "move"
	case "type", "type_text":
		return "type"
	case "keypress", "key_press", "key_combination", "hotkey":
		return "keypress"
	case "scroll":
		return "scroll"
	case "drag", "drag_and_drop":
		return "drag"
	case "wait", "sleep":
		return "wait"
	case "open_url", "navigate", "open_web_browser":
		return "open_url"
	case "search":
		return "search"
	default:
		return action
	}
}

func normalizeComputerBackend(backend string) string {
	backend = strings.TrimSpace(strings.ToLower(backend))
	if backend == "" {
		return "codex_local"
	}
	return backend
}

func computerEnabled(opts ComputerOptions) bool {
	if opts.Enabled {
		return true
	}
	return strings.TrimSpace(opts.Backend) == "" &&
		strings.TrimSpace(opts.CoordinateSpace) == "" &&
		opts.MaxActionsPerTurn == 0 &&
		!opts.ObserveAfterAction &&
		!opts.IncludeWindowsDefault &&
		!opts.RedactTextInAudit &&
		len(opts.AllowedApps) == 0 &&
		len(opts.AllowedDomains) == 0
}

func (c *codexLocalComputerController) ensureActionAllowed(ctx context.Context, action ComputerAction) error {
	if len(c.opts.Computer.AllowedDomains) > 0 {
		switch action.Type {
		case "open_url", "search":
			target := computerActionTargetURL(action)
			if target == "" {
				return nil
			}
			if !computerDomainAllowed(target, c.opts.Computer.AllowedDomains) {
				return fmt.Errorf("computer action blocked for URL domain %q", computerURLHostname(target))
			}
		}
	}
	if len(c.opts.Computer.AllowedApps) == 0 {
		return nil
	}
	switch action.Type {
	case "open_url", "search":
		return fmt.Errorf("computer action %s is blocked while computer.allowed_apps is set; use existing allowed app windows or clear allowed_apps", action.Type)
	case "click", "double_click", "move", "type", "keypress", "scroll", "drag":
		windows, err := runDesktopWindowQuery(ctx, map[string]any{"active_only": true})
		if err != nil {
			return err
		}
		if len(windows) == 0 {
			return fmt.Errorf("active desktop window is required for computer action allowlist")
		}
		if computerWindowAllowed(windows[0], c.opts.Computer.AllowedApps) {
			return nil
		}
		return fmt.Errorf("computer action blocked for active app %q", windows[0].ProcessName)
	default:
		return nil
	}
}

func computerActionTargetURL(action ComputerAction) string {
	switch action.Type {
	case "open_url":
		return strings.TrimSpace(firstNonEmpty(action.URL, action.Query))
	case "search":
		if strings.TrimSpace(action.Query) == "" {
			return "https://www.google.com"
		}
		return "https://www.google.com/search?q=" + url.QueryEscape(strings.TrimSpace(action.Query))
	default:
		return ""
	}
}

func computerDomainAllowed(target string, allowedDomains []string) bool {
	host := computerURLHostname(target)
	if host == "" {
		return false
	}
	for _, allowed := range allowedDomains {
		allowed = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(allowed)), ".")
		if allowed == "" {
			continue
		}
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func computerURLHostname(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Hostname() == "" {
		parsed, err = url.Parse("https://" + target)
		if err != nil {
			return ""
		}
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func computerWindowAllowed(window desktopWindowInfo, allowedApps []string) bool {
	processName := strings.ToLower(strings.TrimSpace(window.ProcessName))
	title := strings.ToLower(strings.TrimSpace(window.Title))
	for _, allowed := range allowedApps {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == "" {
			continue
		}
		if processName == allowed || strings.TrimSuffix(processName, ".exe") == strings.TrimSuffix(allowed, ".exe") {
			return true
		}
		if title != "" && strings.Contains(title, allowed) {
			return true
		}
	}
	return false
}

func computerScreenshotPath(path string) string {
	if path = strings.TrimSpace(path); path != "" {
		return path
	}
	name := "observe-" + time.Now().UTC().Format("20060102-150405.000000000") + ".png"
	return filepath.Join(".anyclaw", "computer", name)
}

func computerScreenshotDimensions(path string) (int, int) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	cfg, err := png.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func computerBoolDefault(input map[string]any, key string, fallback bool) bool {
	if value, ok := input[key]; ok {
		return boolValue(value)
	}
	return fallback
}

func defaultComputerObserveAfterAction(opts ComputerOptions) bool {
	if opts.ObserveAfterAction {
		return true
	}
	return strings.TrimSpace(opts.Backend) == "" &&
		strings.TrimSpace(opts.CoordinateSpace) == "" &&
		opts.MaxActionsPerTurn == 0 &&
		!opts.Enabled
}

func computerMouseArgs(action ComputerAction, x int, y int) map[string]any {
	args := map[string]any{"x": x, "y": y}
	if action.Button != "" {
		args["button"] = action.Button
	}
	if action.HumanLike {
		args["human_like"] = action.HumanLike
	}
	if action.DurationMS > 0 {
		args["duration_ms"] = action.DurationMS
	}
	if action.Steps > 0 {
		args["steps"] = action.Steps
	}
	if action.JitterPX > 0 {
		args["jitter_px"] = action.JitterPX
	}
	if action.SettleMS > 0 {
		args["settle_ms"] = action.SettleMS
	}
	if action.IntervalMS > 0 {
		args["interval_ms"] = action.IntervalMS
	}
	return args
}

func sanitizeComputerAuditInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	return sanitizeComputerAuditValue(input).(map[string]any)
}

func sanitizeComputerAuditValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			lower := strings.ToLower(strings.TrimSpace(key))
			if lower == "text" {
				out[key] = redactedLengthValue(item)
				continue
			}
			if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "clipboard") {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = sanitizeComputerAuditValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeComputerAuditValue(item)
		}
		return out
	case []map[string]any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeComputerAuditValue(item)
		}
		return out
	default:
		return value
	}
}

func redactedLengthValue(value any) string {
	text := fmt.Sprint(value)
	return fmt.Sprintf("[REDACTED len=%d]", len([]rune(text)))
}
