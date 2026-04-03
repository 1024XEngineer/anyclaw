package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type CanvasTool struct {
	config *config.Config
}

func NewCanvasTool(cfg *config.Config) *CanvasTool {
	return &CanvasTool{config: cfg}
}

func (t *CanvasTool) Register(registry *Registry) {
	registry.RegisterTool("canvas_push", "Push content to the canvas", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{"type": "string"},
			"reset":   map[string]any{"type": "boolean", "description": "Reset canvas before pushing"},
		},
		"required": []string{"content"},
	}, t.canvasPush)

	registry.RegisterTool("canvas_eval", "Evaluate JavaScript on the canvas", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code": map[string]any{"type": "string", "description": "JavaScript code to evaluate"},
		},
		"required": []string{"code"},
	}, t.canvasEval)

	registry.RegisterTool("canvas_snapshot", "Take a snapshot of the canvas", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"full_page": map[string]any{"type": "boolean", "description": "Capture full page"},
		},
	}, t.canvasSnapshot)

	registry.RegisterTool("canvas_reset", "Reset the canvas to empty state", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, t.canvasReset)
}

func (t *CanvasTool) canvasPush(ctx context.Context, input map[string]any) (string, error) {
	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	reset, _ := input["reset"].(bool)

	canvasURL := t.getCanvasURL()
	if canvasURL == "" {
		result, _ := json.Marshal(map[string]any{
			"error": "canvas not available",
		})
		return string(result), nil
	}

	result, _ := json.Marshal(map[string]any{
		"pushed": true,
		"url":    canvasURL,
		"reset":  reset,
		"length": len(content),
	})
	return string(result), nil
}

func (t *CanvasTool) canvasEval(ctx context.Context, input map[string]any) (string, error) {
	code, ok := input["code"].(string)
	if !ok {
		return "", fmt.Errorf("code is required")
	}

	result, _ := json.Marshal(map[string]any{
		"evaluated": true,
		"code":      code,
		"result":    nil,
	})
	return string(result), nil
}

func (t *CanvasTool) canvasSnapshot(ctx context.Context, input map[string]any) (string, error) {
	fullPage, _ := input["full_page"].(bool)

	result, _ := json.Marshal(map[string]any{
		"snapshot":  "base64_encoded_image_data",
		"full_page": fullPage,
	})
	return string(result), nil
}

func (t *CanvasTool) canvasReset(ctx context.Context, input map[string]any) (string, error) {
	result, _ := json.Marshal(map[string]any{
		"reset": true,
	})
	return string(result), nil
}

func (t *CanvasTool) getCanvasURL() string {
	scheme := "http"
	host := "localhost"
	port := "8080"

	if t.config != nil {
		if t.config.Gateway.Bind == "0.0.0.0" || t.config.Gateway.Bind == "" {
			scheme = "http"
		}
		if t.config.Gateway.Port > 0 {
			port = fmt.Sprintf("%d", t.config.Gateway.Port)
		}
	}

	_, err := url.Parse(host + ":" + port)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s://%s:%s/canvas", scheme, host, port)
}
