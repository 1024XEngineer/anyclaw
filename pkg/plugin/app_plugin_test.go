package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

func TestRegisterAppPluginsExposesActionTools(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "demo-app")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := Manifest{
		Name:        "demo-app",
		Version:     "1.0.0",
		Description: "Demo app connector",
		Kinds:       []string{"app"},
		Enabled:     true,
		Entrypoint:  "app.py",
		Permissions: []string{"tool:exec"},
		App: &AppSpec{
			Name: "Demo App",
			Actions: []AppActionSpec{
				{
					Name:        "run-task",
					Description: "Run a task",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"task": map[string]any{"type": "string"},
						},
					},
				},
			},
			Workflows: []AppWorkflowSpec{
				{
					Name:        "quick-note",
					Description: "Draft a quick note",
					Action:      "run-task",
					Tags:        []string{"drafting"},
					Defaults:    map[string]any{"task": "Draft a note"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "app.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile entrypoint: %v", err)
	}

	registry, err := NewRegistry(config.PluginsConfig{
		Dir:          baseDir,
		AllowExec:    true,
		RequireTrust: false,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	if got := len(registry.AppRunners(baseDir)); got != 1 {
		t.Fatalf("expected 1 app runner, got %d", got)
	}

	toolRegistry := tools.NewRegistry()
	registry.RegisterAppPlugins(toolRegistry, baseDir, filepath.Join(baseDir, "anyclaw.json"))

	toolName := AppActionToolName("demo-app", "run-task")
	tool, ok := toolRegistry.Get(toolName)
	if !ok {
		t.Fatalf("expected tool %s to be registered", toolName)
	}
	if tool.Description != "Run a task" {
		t.Fatalf("unexpected tool description: %q", tool.Description)
	}

	workflowToolName := AppWorkflowToolName("demo-app", "quick-note")
	workflowTool, ok := toolRegistry.Get(workflowToolName)
	if !ok {
		t.Fatalf("expected workflow tool %s to be registered", workflowToolName)
	}
	if workflowTool.Description != "Draft a quick note" {
		t.Fatalf("unexpected workflow description: %q", workflowTool.Description)
	}
}

func TestResolveWorkflowMatchesPrefersTaggedWorkflow(t *testing.T) {
	registry := &Registry{
		manifests: []Manifest{
			{
				Name:    "image-app",
				Enabled: true,
				App: &AppSpec{
					Name: "Image App",
					Actions: []AppActionSpec{
						{Name: "run"},
					},
					Workflows: []AppWorkflowSpec{
						{
							Name:        "remove-background",
							Description: "Remove the background and export a PNG image",
							Action:      "run",
							Tags:        []string{"image", "background", "png"},
						},
						{
							Name:        "draft-note",
							Description: "Draft a text note",
							Action:      "run",
							Tags:        []string{"text", "note"},
						},
					},
				},
			},
		},
	}

	matches := registry.ResolveWorkflowMatches("remove the background from this photo and export png", 2)
	if len(matches) == 0 {
		t.Fatal("expected at least one workflow match")
	}
	if matches[0].Workflow.ToolName != AppWorkflowToolName("image-app", "remove-background") {
		t.Fatalf("unexpected top workflow: %q", matches[0].Workflow.ToolName)
	}
}

func TestResolveWorkflowMatchesSupportsChineseQQQueries(t *testing.T) {
	registry := &Registry{
		manifests: []Manifest{
			{
				Name:    "qq-local",
				Enabled: true,
				App: &AppSpec{
					Name: "QQ",
					Actions: []AppActionSpec{
						{Name: "send-message"},
						{Name: "reply-current"},
						{Name: "send-file"},
						{Name: "send-image"},
						{Name: "read-current-chat"},
						{Name: "copy-current-chat"},
						{Name: "capture-window"},
						{Name: "ocr-window"},
					},
					Workflows: []AppWorkflowSpec{
						{
							Name:        "send-message",
							Description: "Open QQ and send a message to a contact",
							Action:      "send-message",
							Tags:        []string{"qq", "message", "send", "消息", "发送", "聊天"},
						},
						{
							Name:        "reply-current",
							Description: "Reply in the current QQ chat",
							Action:      "reply-current",
							Tags:        []string{"qq", "reply", "回复", "当前聊天"},
						},
						{
							Name:        "send-file",
							Description: "Open QQ and send a file to the chat",
							Action:      "send-file",
							Tags:        []string{"qq", "file", "attachment", "文件", "发送文件", "附件"},
						},
						{
							Name:        "send-image",
							Description: "Open QQ and send an image to the chat",
							Action:      "send-image",
							Tags:        []string{"qq", "image", "picture", "photo", "发送图片", "图片", "照片"},
						},
						{
							Name:        "read-current-chat",
							Description: "Capture and OCR the current QQ conversation",
							Action:      "read-current-chat",
							Tags:        []string{"qq", "read", "ocr", "聊天记录", "读取聊天", "查看消息"},
						},
						{
							Name:        "copy-current-chat",
							Description: "Copy the current QQ chat content into the clipboard",
							Action:      "copy-current-chat",
							Tags:        []string{"qq", "copy", "clipboard", "chat", "复制聊天", "复制消息", "剪贴板"},
						},
						{
							Name:        "capture-window",
							Description: "Capture the current QQ window to a screenshot file",
							Action:      "capture-window",
							Tags:        []string{"qq", "capture", "screenshot", "window", "截图", "窗口截图"},
						},
						{
							Name:        "ocr-window",
							Description: "Capture and OCR the current QQ window",
							Action:      "ocr-window",
							Tags:        []string{"qq", "ocr", "window", "text", "识别", "文字", "识别文字", "窗口OCR"},
						},
					},
				},
			},
		},
	}

	testCases := []struct {
		query    string
		workflow string
	}{
		{query: "在QQ里给张三发消息", workflow: "send-message"},
		{query: "帮我在QQ里发送文件给小王", workflow: "send-file"},
		{query: "帮我在QQ里发图片给小王", workflow: "send-image"},
		{query: "读取当前QQ聊天记录", workflow: "read-current-chat"},
		{query: "复制当前QQ聊天到剪贴板", workflow: "copy-current-chat"},
		{query: "帮我截图当前QQ窗口", workflow: "capture-window"},
		{query: "识别QQ窗口里的文字", workflow: "ocr-window"},
	}

	for _, testCase := range testCases {
		matches := registry.ResolveWorkflowMatches(testCase.query, 2)
		if len(matches) == 0 {
			t.Fatalf("expected QQ workflow match for query %q", testCase.query)
		}
		want := AppWorkflowToolName("qq-local", testCase.workflow)
		if matches[0].Workflow.ToolName != want {
			t.Fatalf("unexpected top QQ workflow for query %q: got %q want %q", testCase.query, matches[0].Workflow.ToolName, want)
		}
	}
}

func TestResolveWorkflowMatchesWithPairingsUsesPairingHints(t *testing.T) {
	registry := &Registry{
		manifests: []Manifest{
			{
				Name:    "image-app",
				Enabled: true,
				App: &AppSpec{
					Name: "Image App",
					Actions: []AppActionSpec{
						{Name: "run"},
					},
					Workflows: []AppWorkflowSpec{
						{
							Name:        "remove-background",
							Description: "Remove the background from an image",
							Action:      "run",
							Tags:        []string{"image", "background"},
						},
					},
				},
			},
		},
	}

	matches := registry.ResolveWorkflowMatchesWithPairings("帮我给这张图抠图", 2, []*apps.Pairing{
		{
			ID:       "pair-1",
			App:      "image-app",
			Workflow: "remove-background",
			Name:     "local-cutout",
			Binding:  "primary",
			Triggers: []string{"抠图", "去背景"},
			Defaults: map[string]any{"export_format": "png"},
		},
	})
	if len(matches) == 0 {
		t.Fatal("expected pairing-aware workflow match")
	}
	if matches[0].Workflow.ToolName != AppWorkflowToolName("image-app", "remove-background") {
		t.Fatalf("unexpected workflow tool: %q", matches[0].Workflow.ToolName)
	}
	if matches[0].Pairing == nil || matches[0].Pairing.Name != "local-cutout" {
		t.Fatalf("expected pairing info in match, got %#v", matches[0].Pairing)
	}
	if matches[0].Pairing.Binding != "primary" {
		t.Fatalf("expected pairing binding to round-trip, got %#v", matches[0].Pairing)
	}
}
