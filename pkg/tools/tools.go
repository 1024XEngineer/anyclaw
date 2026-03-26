package tools

import "context"

func RegisterBuiltins(r *Registry, opts BuiltinOptions) {
	RegisterFileTools(r, opts)
	RegisterWebTools(r, opts)
	RegisterDesktopTools(r, opts)
}

func RegisterFileTools(r *Registry, opts BuiltinOptions) {
	workingDir := opts.WorkingDir
	r.RegisterTool(
		"read_file",
		"Read the contents of a file from the filesystem",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{"type": "string", "description": "Path to the file"},
			},
			"required": []string{"path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "read_file", input, func(ctx context.Context, input map[string]any) (string, error) {
				return ReadFileToolWithCwd(ctx, input, workingDir)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"write_file",
		"Write content to a file",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]string{"type": "string", "description": "Path to the file"},
				"content": map[string]string{"type": "string", "description": "Content to write"},
			},
			"required": []string{"path", "content"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "write_file", input, func(ctx context.Context, input map[string]any) (string, error) {
				return WriteFileToolWithPolicy(ctx, input, workingDir, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"list_directory",
		"List files in a directory",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{"type": "string", "description": "Path to directory"},
			},
			"required": []string{"path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "list_directory", input, func(ctx context.Context, input map[string]any) (string, error) {
				return ListDirectoryToolWithCwd(ctx, input, workingDir)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"search_files",
		"Search for files matching a pattern",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]string{"type": "string", "description": "Root path to search"},
				"pattern": map[string]string{"type": "string", "description": "Search pattern"},
			},
			"required": []string{"path", "pattern"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "search_files", input, func(ctx context.Context, input map[string]any) (string, error) {
				return SearchFilesToolWithCwd(ctx, input, workingDir)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"run_command",
		"Execute a shell command within the working directory",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]string{"type": "string", "description": "Shell command to execute"},
				"cwd":     map[string]string{"type": "string", "description": "Optional working directory override"},
				"shell":   map[string]string{"type": "string", "description": "Optional shell: auto, cmd, powershell, pwsh, sh, or bash"},
			},
			"required": []string{"command"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "run_command", input, func(ctx context.Context, input map[string]any) (string, error) {
				return RunCommandToolWithPolicy(ctx, input, opts)
			})(ctx, input)
		},
	)
}

func RegisterWebTools(r *Registry, opts BuiltinOptions) {
	r.RegisterTool(
		"web_search",
		"Search the web for information using DuckDuckGo",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]string{"type": "string", "description": "Search query"},
				"max_results": map[string]string{"type": "number", "description": "Maximum number of results (default: 5)"},
			},
			"required": []string{"query"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "web_search", input, WebSearchTool)(ctx, input)
		},
	)

	r.RegisterTool(
		"fetch_url",
		"Fetch and extract text content from a URL",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]string{"type": "string", "description": "URL to fetch"},
			},
			"required": []string{"url"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "fetch_url", input, FetchURLTool)(ctx, input)
		},
	)

	r.RegisterTool(
		"browser_navigate",
		"Open a page in a browser automation session",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"url":        map[string]string{"type": "string", "description": "URL to open"},
			},
			"required": []string{"url"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserNavigateTool(ctx, input)
		},
	)

	r.RegisterTool(
		"browser_click",
		"Click an element on the current page",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "CSS selector to click"},
			},
			"required": []string{"selector"},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserClickTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_type",
		"Type text into an element",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "CSS selector to type into"},
				"text":       map[string]string{"type": "string", "description": "Text to type"},
			},
			"required": []string{"selector", "text"},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserTypeTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_screenshot",
		"Take a screenshot of the current page or element",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"path":       map[string]string{"type": "string", "description": "File path to save screenshot"},
				"selector":   map[string]string{"type": "string", "description": "Optional CSS selector for element screenshot"},
			},
			"required": []string{"path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserScreenshotTool(ctx, input)
		},
	)

	r.RegisterTool(
		"browser_upload",
		"Upload a file via input element",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "File input CSS selector"},
				"path":       map[string]string{"type": "string", "description": "Local path to upload"},
			},
			"required": []string{"selector", "path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserUploadTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_wait",
		"Wait for an element or page state",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "Optional CSS selector"},
				"state":      map[string]string{"type": "string", "description": "ready, visible, or enabled"},
				"timeout_ms": map[string]string{"type": "number", "description": "Timeout in milliseconds"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserWaitTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_select",
		"Select a value in a form control",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "CSS selector"},
				"value":      map[string]string{"type": "string", "description": "Value to set"},
			},
			"required": []string{"selector", "value"},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserSelectTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_press",
		"Press a keyboard key in the page",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "Optional CSS selector to focus"},
				"key":        map[string]string{"type": "string", "description": "Key to press, e.g. Enter, Tab, ArrowDown"},
			},
			"required": []string{"key"},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserPressTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_scroll",
		"Scroll the page or an element",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "Optional CSS selector to scroll inside"},
				"direction":  map[string]string{"type": "string", "description": "up or down"},
				"pixels":     map[string]string{"type": "number", "description": "Scroll distance in pixels"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserScrollTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_download",
		"Download a linked resource to disk",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"selector":   map[string]string{"type": "string", "description": "Optional selector whose href/src should be downloaded"},
				"url":        map[string]string{"type": "string", "description": "Optional absolute URL to download"},
				"path":       map[string]string{"type": "string", "description": "Destination file path"},
			},
			"required": []string{"path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserDownloadTool(ctx, input)
		},
	)

	r.RegisterTool(
		"browser_snapshot",
		"Capture the current page HTML and title",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserSnapshotTool(ctx, input)
		},
	)

	r.RegisterTool(
		"browser_eval",
		"Evaluate JavaScript in the page context",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"expression": map[string]string{"type": "string", "description": "JavaScript expression"},
			},
			"required": []string{"expression"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserEvaluateTool(ctx, input)
		},
	)

	r.RegisterTool(
		"browser_pdf",
		"Export the current page to PDF",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional tab id"},
				"path":       map[string]string{"type": "string", "description": "File path to save PDF"},
			},
			"required": []string{"path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserPDFTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_close",
		"Close a browser automation session",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserCloseTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_tab_new",
		"Create a new browser tab in the session",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Optional desired tab id"},
				"url":        map[string]string{"type": "string", "description": "Optional URL to open immediately"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserTabNewTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_tab_list",
		"List all tabs in the browser session",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) { return BrowserTabListTool(ctx, input) },
	)

	r.RegisterTool(
		"browser_tab_switch",
		"Switch the active browser tab",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Tab id to activate"},
			},
			"required": []string{"tab_id"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserTabSwitchTool(ctx, input)
		},
	)

	r.RegisterTool(
		"browser_tab_close",
		"Close a specific browser tab",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]string{"type": "string", "description": "Browser session id"},
				"tab_id":     map[string]string{"type": "string", "description": "Tab id to close"},
			},
			"required": []string{"tab_id"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserTabCloseTool(ctx, input)
		},
	)
}

func RegisterDesktopTools(r *Registry, opts BuiltinOptions) {
	r.RegisterTool(
		"desktop_open",
		"Open an application, URL, or file on the desktop host",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]string{"type": "string", "description": "Application path/name, URL, or file path"},
				"kind":   map[string]string{"type": "string", "description": "Optional kind: app, url, or file"},
			},
			"required": []string{"target"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_open", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopOpenTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_type",
		"Type text into the active desktop window",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]string{"type": "string", "description": "Text to send to the active window"},
			},
			"required": []string{"text"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_type", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopTypeTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_hotkey",
		"Send a desktop hotkey chord to the active window",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keys": map[string]any{
					"type":        "array",
					"description": "List of keys, e.g. [\"ctrl\", \"s\"]",
					"items":       map[string]string{"type": "string"},
				},
			},
			"required": []string{"keys"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_hotkey", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopHotkeyTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_click",
		"Click a desktop screen coordinate on the host",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x":      map[string]string{"type": "number", "description": "Screen X coordinate"},
				"y":      map[string]string{"type": "number", "description": "Screen Y coordinate"},
				"button": map[string]string{"type": "string", "description": "Optional mouse button: left, right, middle"},
			},
			"required": []string{"x", "y"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_click", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopClickTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_screenshot",
		"Capture a screenshot of the desktop and save it to a file",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{"type": "string", "description": "Destination PNG path inside the working directory"},
			},
			"required": []string{"path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_screenshot", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopScreenshotTool(ctx, input, opts)
			})(ctx, input)
		},
	)
}

func auditCall(opts BuiltinOptions, toolName string, input map[string]any, next ToolFunc) ToolFunc {
	return func(ctx context.Context, _ map[string]any) (string, error) {
		output, err := next(ctx, input)
		if opts.AuditLogger != nil {
			opts.AuditLogger.LogTool(toolName, input, output, err)
		}
		return output, err
	}
}
