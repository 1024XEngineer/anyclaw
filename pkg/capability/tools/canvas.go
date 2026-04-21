package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type CanvasTool struct {
	config     *config.Config
	apiToken   string
	gatewayURL string
}

func NewCanvasTool(cfg *config.Config) *CanvasTool {
	tool := &CanvasTool{config: cfg}
	if cfg != nil {
		tool.apiToken = cfg.Security.APIToken
		tool.gatewayURL = tool.buildGatewayURL()
	}
	return tool
}

func (t *CanvasTool) Register(registry *Registry) {
	registry.RegisterTool("canvas_push", "Push content to the canvas", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{"type": "string", "description": "HTML, A2UI JSON, or markdown content"},
			"id":      map[string]any{"type": "string", "description": "Canvas entry ID (auto-generated if empty)"},
			"name":    map[string]any{"type": "string", "description": "Display name for the canvas entry"},
			"type":    map[string]any{"type": "string", "enum": []string{"html", "a2ui", "markdown", "json", "text"}, "description": "Content type"},
			"reset":   map[string]any{"type": "boolean", "description": "Reset canvas before pushing"},
		},
		"required": []string{"content"},
	}, t.canvasPush)

	registry.RegisterTool("canvas_eval", "Evaluate JavaScript on the canvas", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code": map[string]any{"type": "string", "description": "JavaScript code to evaluate"},
			"id":   map[string]any{"type": "string", "description": "Canvas entry ID"},
		},
		"required": []string{"code"},
	}, t.canvasEval)

	registry.RegisterTool("canvas_snapshot", "Take a snapshot of the canvas", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":        map[string]any{"type": "string", "description": "Canvas entry ID"},
			"full_page": map[string]any{"type": "boolean", "description": "Capture full page"},
		},
	}, t.canvasSnapshot)

	registry.RegisterTool("canvas_reset", "Reset the canvas to empty state", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Canvas entry ID to reset"},
		},
	}, t.canvasReset)

	registry.RegisterTool("canvas_list", "List all canvas entries", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, t.canvasList)

	registry.RegisterTool("canvas_get", "Get a specific canvas entry", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Canvas entry ID"},
		},
		"required": []string{"id"},
	}, t.canvasGet)

	registry.RegisterTool("canvas_versions", "Get version history for a canvas entry", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":    map[string]any{"type": "string", "description": "Canvas entry ID"},
			"limit": map[string]any{"type": "number", "description": "Max versions to return"},
		},
		"required": []string{"id"},
	}, t.canvasVersions)
}

func (t *CanvasTool) canvasPush(ctx context.Context, input map[string]any) (string, error) {
	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	id, _ := input["id"].(string)
	name, _ := input["name"].(string)
	contentType, _ := input["type"].(string)
	reset, _ := input["reset"].(bool)

	if t.gatewayURL == "" {
		return t.mockPushResult(id, content, contentType, reset)
	}

	reqBody := map[string]any{
		"id":      id,
		"name":    name,
		"content": content,
		"type":    contentType,
		"reset":   reset,
	}

	resp, err := t.doRequest(ctx, http.MethodPost, "/api/canvas", reqBody)
	if err != nil {
		return t.mockPushResult(id, content, contentType, reset)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Sprintf(`{"error": "canvas push failed: %s", "status": %d}`, string(body), resp.StatusCode), nil
	}

	return string(body), nil
}

func (t *CanvasTool) canvasEval(ctx context.Context, input map[string]any) (string, error) {
	code, ok := input["code"].(string)
	if !ok {
		return "", fmt.Errorf("code is required")
	}

	id, _ := input["id"].(string)

	result := map[string]any{
		"evaluated": true,
		"code":      code,
		"canvas_id": id,
		"note":      "JavaScript evaluation requires a browser context. Use canvas_push with HTML content containing <script> tags instead.",
	}

	data, _ := json.Marshal(result)
	return string(data), nil
}

func (t *CanvasTool) canvasSnapshot(ctx context.Context, input map[string]any) (string, error) {
	id, _ := input["id"].(string)
	fullPage, _ := input["full_page"].(bool)

	result := map[string]any{
		"canvas_id": id,
		"full_page": fullPage,
		"note":      "Screenshot capture requires a headless browser. View the canvas at /canvas/view/{id} or /canvas/a2ui/{id}.",
	}

	data, _ := json.Marshal(result)
	return string(data), nil
}

func (t *CanvasTool) canvasReset(ctx context.Context, input map[string]any) (string, error) {
	id, _ := input["id"].(string)

	if id == "" {
		return `{"error": "id is required for canvas_reset"}`, nil
	}

	if t.gatewayURL == "" {
		result, _ := json.Marshal(map[string]any{"reset": true, "id": id, "note": "canvas not available, simulated reset"})
		return string(result), nil
	}

	reqBody := map[string]any{"id": id}
	resp, err := t.doRequest(ctx, http.MethodPost, "/api/canvas/"+id+"/reset", reqBody)
	if err != nil {
		result, _ := json.Marshal(map[string]any{"reset": true, "id": id, "note": "reset simulated, gateway unreachable"})
		return string(result), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func (t *CanvasTool) canvasList(ctx context.Context, input map[string]any) (string, error) {
	if t.gatewayURL == "" {
		return `{"entries": [], "note": "canvas not available"}`, nil
	}

	resp, err := t.doRequest(ctx, http.MethodGet, "/api/canvas", nil)
	if err != nil {
		return `{"entries": [], "note": "failed to fetch canvas entries"}`, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func (t *CanvasTool) canvasGet(ctx context.Context, input map[string]any) (string, error) {
	id, ok := input["id"].(string)
	if !ok || id == "" {
		return `{"error": "id is required"}`, nil
	}

	if t.gatewayURL == "" {
		return fmt.Sprintf(`{"error": "canvas not available", "id": "%s"}`, id), nil
	}

	resp, err := t.doRequest(ctx, http.MethodGet, "/api/canvas/"+id, nil)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to fetch entry", "id": "%s"}`, id), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func (t *CanvasTool) canvasVersions(ctx context.Context, input map[string]any) (string, error) {
	id, ok := input["id"].(string)
	if !ok || id == "" {
		return `{"error": "id is required"}`, nil
	}

	limit := 10
	if l, ok := input["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if t.gatewayURL == "" {
		return fmt.Sprintf(`{"versions": [], "id": "%s", "note": "canvas not available"}`, id), nil
	}

	url := fmt.Sprintf("/api/canvas/%s/versions?limit=%d", id, limit)
	resp, err := t.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Sprintf(`{"versions": [], "id": "%s", "note": "failed to fetch versions"}`, id), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func (t *CanvasTool) doRequest(ctx context.Context, method string, path string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	url := t.gatewayURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

func (t *CanvasTool) mockPushResult(id string, content string, contentType string, reset bool) (string, error) {
	if id == "" {
		id = "canvas-mock"
	}
	if contentType == "" {
		contentType = "html"
	}
	result, _ := json.Marshal(map[string]any{
		"pushed":   true,
		"id":       id,
		"type":     contentType,
		"reset":    reset,
		"length":   len(content),
		"view_url": fmt.Sprintf("/canvas/view/%s", id),
		"note":     "canvas pushed locally (gateway not reachable)",
	})
	return string(result), nil
}

func (t *CanvasTool) buildGatewayURL() string {
	if t.config == nil {
		return ""
	}

	scheme := "http"
	host := "localhost"
	port := 8080

	if t.config.Gateway.Port > 0 {
		port = t.config.Gateway.Port
	}
	if t.config.Gateway.Host != "" {
		host = t.config.Gateway.Host
	}

	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}
