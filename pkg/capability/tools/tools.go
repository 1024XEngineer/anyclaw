package tools

import (
	"context"

	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

func RegisterBuiltins(r *Registry, opts BuiltinOptions) {
	RegisterFileTools(r, opts)
	RegisterMemoryTools(r, opts)
	RegisterQMDTools(r, opts)
	RegisterWebTools(r, opts)
	RegisterDesktopTools(r, opts)
	RegisterCLIHubTools(r, opts)
	RegisterClawBridgeTools(r, opts)
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
				return ReadFileToolWithPolicy(ctx, input, workingDir, opts)
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
				return ListDirectoryToolWithPolicy(ctx, input, workingDir, opts)
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
				return SearchFilesToolWithPolicy(ctx, input, workingDir, opts)
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

func RegisterMemoryTools(r *Registry, opts BuiltinOptions) {
	workingDir := opts.WorkingDir

	r.RegisterTool(
		"memory_search",
		"Search memory entries. Uses the memory backend (SQLite/dual) when available, falls back to daily memory files.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]string{"type": "string", "description": "Text to search for in memory entries"},
				"limit": map[string]string{"type": "number", "description": "Maximum number of matches to return"},
				"date":  map[string]string{"type": "string", "description": "Optional day filter for daily files: YYYY-MM-DD, today, yesterday, or latest"},
			},
			"required": []string{"query"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "memory_search", input, func(ctx context.Context, input map[string]any) (string, error) {
				return MemorySearchToolWithBackend(ctx, input, workingDir, opts.MemoryBackend)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"memory_vector_search",
		"Search memory entries using vector embeddings. Requires a vector-capable memory backend.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":     map[string]string{"type": "string", "description": "Text query (used as fallback if no embedding provided)"},
				"limit":     map[string]string{"type": "number", "description": "Maximum number of matches to return"},
				"threshold": map[string]string{"type": "number", "description": "Minimum cosine similarity threshold (default: 0.5)"},
				"embedding": map[string]string{"type": "array", "description": "Query embedding vector (optional, falls back to text search)"},
			},
			"required": []string{"query"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "memory_vector_search", input, func(ctx context.Context, input map[string]any) (string, error) {
				if vec, ok := opts.MemoryBackend.(memory.VectorBackend); ok {
					return MemoryVectorSearchTool(ctx, input, opts.MemoryBackend, vec)
				}
				return MemorySearchToolWithBackend(ctx, input, workingDir, opts.MemoryBackend)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"memory_hybrid_search",
		"Search memory entries using combined text + vector scoring. Requires a vector-capable memory backend.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":         map[string]string{"type": "string", "description": "Text query for hybrid search"},
				"limit":         map[string]string{"type": "number", "description": "Maximum number of matches to return"},
				"vector_weight": map[string]string{"type": "number", "description": "Weight for vector score vs text score (0.0-1.0, default: 0.5)"},
				"embedding":     map[string]string{"type": "array", "description": "Query embedding vector (required for hybrid)"},
			},
			"required": []string{"query"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "memory_hybrid_search", input, func(ctx context.Context, input map[string]any) (string, error) {
				if vec, ok := opts.MemoryBackend.(memory.VectorBackend); ok {
					return MemoryHybridSearchTool(ctx, input, opts.MemoryBackend, vec)
				}
				return MemorySearchToolWithBackend(ctx, input, workingDir, opts.MemoryBackend)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"memory_get",
		"Read a specific daily workspace memory file from memory/*.md",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]string{"type": "string", "description": "Target day: YYYY-MM-DD, today, yesterday, or latest"},
			},
			"required": []string{"date"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "memory_get", input, func(ctx context.Context, input map[string]any) (string, error) {
				return MemoryGetToolWithCwd(ctx, input, workingDir)
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
		"Open a page in a headless browser automation session (not a visible desktop browser window)",
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
			return BrowserScreenshotToolWithPolicy(ctx, input, opts)
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
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserUploadToolWithPolicy(ctx, input, opts)
		},
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
			return BrowserDownloadToolWithPolicy(ctx, input, opts)
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
		func(ctx context.Context, input map[string]any) (string, error) {
			return BrowserPDFToolWithPolicy(ctx, input, opts)
		},
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
		"Open a visible application, URL, or file on the desktop host",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]string{"type": "string", "description": "Application path/name, URL, or file path. Use this to open a real browser window."},
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
		"desktop_type_human",
		"Type text into the active desktop window with small delays to resemble human input",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":        map[string]string{"type": "string", "description": "Text to send to the active window"},
				"delay_ms":    map[string]string{"type": "number", "description": "Base delay between characters"},
				"jitter_ms":   map[string]string{"type": "number", "description": "Additional random per-character delay"},
				"pause_every": map[string]string{"type": "number", "description": "Insert a longer pause after this many characters"},
				"pause_ms":    map[string]string{"type": "number", "description": "Duration of the longer pause"},
				"submit":      map[string]string{"type": "boolean", "description": "Whether to press Enter after typing"},
			},
			"required": []string{"text"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_type_human", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopTypeHumanTool(ctx, input, opts)
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
		"desktop_clipboard_set",
		"Set text into the Windows clipboard",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]string{"type": "string", "description": "Text to place on the clipboard"},
			},
			"required": []string{"text"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_clipboard_set", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopClipboardSetTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_clipboard_get",
		"Read text from the Windows clipboard",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_clipboard_get", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopClipboardGetTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_paste",
		"Paste the current clipboard text, or set clipboard text and paste it into the active window",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":    map[string]string{"type": "string", "description": "Optional text to place on the clipboard before pasting"},
				"wait_ms": map[string]string{"type": "number", "description": "Optional pause before sending Ctrl+V"},
				"submit":  map[string]string{"type": "boolean", "description": "Whether to press Enter after pasting"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_paste", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopPasteTool(ctx, input, opts)
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
		"desktop_move",
		"Move the mouse cursor to a desktop screen coordinate",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x": map[string]string{"type": "number", "description": "Screen X coordinate"},
				"y": map[string]string{"type": "number", "description": "Screen Y coordinate"},
			},
			"required": []string{"x", "y"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_move", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopMoveTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_double_click",
		"Double click a desktop screen coordinate on the host",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x":           map[string]string{"type": "number", "description": "Screen X coordinate"},
				"y":           map[string]string{"type": "number", "description": "Screen Y coordinate"},
				"button":      map[string]string{"type": "string", "description": "Optional mouse button: left, right, middle"},
				"interval_ms": map[string]string{"type": "number", "description": "Delay between clicks in milliseconds"},
			},
			"required": []string{"x", "y"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_double_click", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopDoubleClickTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_scroll",
		"Scroll the mouse wheel on the desktop host",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x":         map[string]string{"type": "number", "description": "Optional screen X coordinate"},
				"y":         map[string]string{"type": "number", "description": "Optional screen Y coordinate"},
				"direction": map[string]string{"type": "string", "description": "Optional direction: up or down"},
				"clicks":    map[string]string{"type": "number", "description": "Optional wheel clicks when direction is used"},
				"delta":     map[string]string{"type": "number", "description": "Optional raw wheel delta override"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_scroll", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopScrollTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_drag",
		"Drag the mouse from one desktop coordinate to another",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x1":          map[string]string{"type": "number", "description": "Starting screen X coordinate"},
				"y1":          map[string]string{"type": "number", "description": "Starting screen Y coordinate"},
				"x2":          map[string]string{"type": "number", "description": "Ending screen X coordinate"},
				"y2":          map[string]string{"type": "number", "description": "Ending screen Y coordinate"},
				"button":      map[string]string{"type": "string", "description": "Optional mouse button: left, right, middle"},
				"steps":       map[string]string{"type": "number", "description": "Optional number of interpolation steps"},
				"duration_ms": map[string]string{"type": "number", "description": "Optional drag duration in milliseconds"},
			},
			"required": []string{"x1", "y1", "x2", "y2"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_drag", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopDragTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_wait",
		"Pause desktop execution for a fixed number of milliseconds",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"wait_ms": map[string]string{"type": "number", "description": "Milliseconds to wait"},
			},
			"required": []string{"wait_ms"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_wait", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopWaitTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_list_windows",
		"List desktop application windows on the local host",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":        map[string]string{"type": "string", "description": "Optional window title filter"},
				"process_name": map[string]string{"type": "string", "description": "Optional process name filter, without .exe"},
				"handle":       map[string]string{"type": "number", "description": "Optional native window handle filter"},
				"match":        map[string]string{"type": "string", "description": "Optional title match mode: contains or exact"},
				"active_only":  map[string]string{"type": "boolean", "description": "Whether to return only the focused window"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_list_windows", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopListWindowsTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_wait_window",
		"Wait until a desktop application window appears on the local host",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":        map[string]string{"type": "string", "description": "Optional window title filter"},
				"process_name": map[string]string{"type": "string", "description": "Optional process name filter, without .exe"},
				"handle":       map[string]string{"type": "number", "description": "Optional native window handle filter"},
				"match":        map[string]string{"type": "string", "description": "Optional title match mode: contains or exact"},
				"active_only":  map[string]string{"type": "boolean", "description": "Whether to wait for a focused window"},
				"timeout_ms":   map[string]string{"type": "number", "description": "Optional timeout in milliseconds"},
				"interval_ms":  map[string]string{"type": "number", "description": "Optional poll interval in milliseconds"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_wait_window", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopWaitWindowTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_focus_window",
		"Bring a desktop window to the foreground by title or process name",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":        map[string]string{"type": "string", "description": "Window title to match"},
				"process_name": map[string]string{"type": "string", "description": "Process name to match, without .exe"},
				"match":        map[string]string{"type": "string", "description": "Optional title match mode: contains or exact"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_focus_window", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopFocusWindowTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_inspect_ui",
		"Inspect UI automation elements inside a desktop application window",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":             map[string]string{"type": "string", "description": "Window title to match"},
				"process_name":      map[string]string{"type": "string", "description": "Process name to match, without .exe"},
				"handle":            map[string]string{"type": "number", "description": "Optional native window handle"},
				"match":             map[string]string{"type": "string", "description": "Optional match mode for title/name/class filters: contains or exact"},
				"scope":             map[string]string{"type": "string", "description": "children or descendants"},
				"name":              map[string]string{"type": "string", "description": "Optional visible control name filter"},
				"automation_id":     map[string]string{"type": "string", "description": "Optional automation id filter"},
				"class_name":        map[string]string{"type": "string", "description": "Optional class name filter"},
				"control_type":      map[string]string{"type": "string", "description": "Optional control type filter, e.g. button or edit"},
				"max_elements":      map[string]string{"type": "number", "description": "Optional maximum number of matching elements"},
				"include_offscreen": map[string]string{"type": "boolean", "description": "Whether to include offscreen controls"},
				"include_disabled":  map[string]string{"type": "boolean", "description": "Whether to include disabled controls"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_inspect_ui", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopInspectUITool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_invoke_ui",
		"Invoke or click a UI automation control inside a desktop application window",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":             map[string]string{"type": "string", "description": "Window title to match"},
				"process_name":      map[string]string{"type": "string", "description": "Process name to match, without .exe"},
				"handle":            map[string]string{"type": "number", "description": "Optional native window handle"},
				"match":             map[string]string{"type": "string", "description": "Optional match mode for title/name/class filters: contains or exact"},
				"scope":             map[string]string{"type": "string", "description": "children or descendants"},
				"name":              map[string]string{"type": "string", "description": "Optional visible control name filter"},
				"automation_id":     map[string]string{"type": "string", "description": "Optional automation id filter"},
				"class_name":        map[string]string{"type": "string", "description": "Optional class name filter"},
				"control_type":      map[string]string{"type": "string", "description": "Optional control type filter, e.g. button or edit"},
				"index":             map[string]string{"type": "number", "description": "Optional 1-based match index"},
				"action":            map[string]string{"type": "string", "description": "auto, invoke, click, focus, select, expand, collapse, or toggle"},
				"include_offscreen": map[string]string{"type": "boolean", "description": "Whether to include offscreen controls"},
				"include_disabled":  map[string]string{"type": "boolean", "description": "Whether to include disabled controls"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_invoke_ui", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopInvokeUITool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_set_value_ui",
		"Set the value of a UI automation input control inside a desktop application window",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":             map[string]string{"type": "string", "description": "Window title to match"},
				"process_name":      map[string]string{"type": "string", "description": "Process name to match, without .exe"},
				"handle":            map[string]string{"type": "number", "description": "Optional native window handle"},
				"match":             map[string]string{"type": "string", "description": "Optional match mode for title/name/class filters: contains or exact"},
				"scope":             map[string]string{"type": "string", "description": "children or descendants"},
				"name":              map[string]string{"type": "string", "description": "Optional visible control name filter"},
				"automation_id":     map[string]string{"type": "string", "description": "Optional automation id filter"},
				"class_name":        map[string]string{"type": "string", "description": "Optional class name filter"},
				"control_type":      map[string]string{"type": "string", "description": "Optional control type filter, e.g. edit or document"},
				"index":             map[string]string{"type": "number", "description": "Optional 1-based match index"},
				"value":             map[string]string{"type": "string", "description": "Text value to enter"},
				"append":            map[string]string{"type": "boolean", "description": "Whether to append instead of replace"},
				"submit":            map[string]string{"type": "boolean", "description": "Whether to press Enter after setting the value"},
				"include_offscreen": map[string]string{"type": "boolean", "description": "Whether to include offscreen controls"},
				"include_disabled":  map[string]string{"type": "boolean", "description": "Whether to include disabled controls"},
			},
			"required": []string{"value"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_set_value_ui", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopSetValueUITool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_resolve_target",
		"Resolve a local desktop target by combining window, UI automation, OCR text, and image matching",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"strategy":          map[string]string{"type": "string", "description": "Optional single strategy: auto, window, ui, text, or image"},
				"strategies":        map[string]string{"type": "array", "description": "Optional ordered strategy list: window, ui, text, image"},
				"title":             map[string]string{"type": "string", "description": "Optional window title to match"},
				"process_name":      map[string]string{"type": "string", "description": "Optional process name to match, without .exe"},
				"handle":            map[string]string{"type": "number", "description": "Optional native window handle"},
				"match":             map[string]string{"type": "string", "description": "Optional title/name/class match mode: contains or exact"},
				"scope":             map[string]string{"type": "string", "description": "Optional UI automation scope: children or descendants"},
				"name":              map[string]string{"type": "string", "description": "Optional UI automation control name"},
				"automation_id":     map[string]string{"type": "string", "description": "Optional UI automation AutomationId"},
				"class_name":        map[string]string{"type": "string", "description": "Optional UI automation class name"},
				"control_type":      map[string]string{"type": "string", "description": "Optional UI automation control type"},
				"index":             map[string]string{"type": "number", "description": "Optional 1-based UI match index"},
				"include_offscreen": map[string]string{"type": "boolean", "description": "Whether to include offscreen UI controls"},
				"include_disabled":  map[string]string{"type": "boolean", "description": "Whether to include disabled UI controls"},
				"text":              map[string]string{"type": "string", "description": "Optional OCR text to find"},
				"mode":              map[string]string{"type": "string", "description": "Optional OCR match mode: contains, exact, or regex"},
				"ignore_case":       map[string]string{"type": "boolean", "description": "Whether OCR matching ignores case"},
				"occurrence":        map[string]string{"type": "number", "description": "Optional 1-based OCR match occurrence"},
				"min_confidence":    map[string]string{"type": "number", "description": "Optional minimum OCR confidence"},
				"lang":              map[string]string{"type": "string", "description": "Optional Tesseract language code"},
				"psm":               map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":               map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
				"template_path":     map[string]string{"type": "string", "description": "Optional template image path"},
				"threshold":         map[string]string{"type": "number", "description": "Optional image similarity threshold between 0 and 1"},
				"path":              map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"require_found":     map[string]string{"type": "boolean", "description": "Whether to return an error when the target cannot be resolved"},
				"crop_to_window":    map[string]string{"type": "boolean", "description": "Whether to crop vision matching to the selected window"},
				"search_x":          map[string]string{"type": "number", "description": "Optional search area X coordinate for image matching"},
				"search_y":          map[string]string{"type": "number", "description": "Optional search area Y coordinate for image matching"},
				"search_width":      map[string]string{"type": "number", "description": "Optional search area width for image matching"},
				"search_height":     map[string]string{"type": "number", "description": "Optional search area height for image matching"},
				"step":              map[string]string{"type": "number", "description": "Optional coarse image search step in pixels"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_resolve_target", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopResolveTargetTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_activate_target",
		"Activate a resolved desktop target by invoking a UI control or clicking the matched text, image, or window",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"strategy":          map[string]string{"type": "string", "description": "Optional single strategy: auto, window, ui, text, or image"},
				"strategies":        map[string]string{"type": "array", "description": "Optional ordered strategy list: window, ui, text, image"},
				"action":            map[string]string{"type": "string", "description": "Optional action: auto, click, double_click, focus, invoke, select, expand, collapse, or toggle"},
				"button":            map[string]string{"type": "string", "description": "Optional mouse button for click-based fallback: left, right, middle"},
				"interval_ms":       map[string]string{"type": "number", "description": "Optional double click interval in milliseconds"},
				"title":             map[string]string{"type": "string", "description": "Optional window title to match"},
				"process_name":      map[string]string{"type": "string", "description": "Optional process name to match, without .exe"},
				"handle":            map[string]string{"type": "number", "description": "Optional native window handle"},
				"match":             map[string]string{"type": "string", "description": "Optional title/name/class match mode: contains or exact"},
				"scope":             map[string]string{"type": "string", "description": "Optional UI automation scope: children or descendants"},
				"name":              map[string]string{"type": "string", "description": "Optional UI automation control name"},
				"automation_id":     map[string]string{"type": "string", "description": "Optional UI automation AutomationId"},
				"class_name":        map[string]string{"type": "string", "description": "Optional UI automation class name"},
				"control_type":      map[string]string{"type": "string", "description": "Optional UI automation control type"},
				"index":             map[string]string{"type": "number", "description": "Optional 1-based UI match index"},
				"include_offscreen": map[string]string{"type": "boolean", "description": "Whether to include offscreen UI controls"},
				"include_disabled":  map[string]string{"type": "boolean", "description": "Whether to include disabled UI controls"},
				"text":              map[string]string{"type": "string", "description": "Optional OCR text to find"},
				"mode":              map[string]string{"type": "string", "description": "Optional OCR match mode: contains, exact, or regex"},
				"ignore_case":       map[string]string{"type": "boolean", "description": "Whether OCR matching ignores case"},
				"occurrence":        map[string]string{"type": "number", "description": "Optional 1-based OCR match occurrence"},
				"min_confidence":    map[string]string{"type": "number", "description": "Optional minimum OCR confidence"},
				"lang":              map[string]string{"type": "string", "description": "Optional Tesseract language code"},
				"psm":               map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":               map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
				"template_path":     map[string]string{"type": "string", "description": "Optional template image path"},
				"threshold":         map[string]string{"type": "number", "description": "Optional image similarity threshold between 0 and 1"},
				"path":              map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"crop_to_window":    map[string]string{"type": "boolean", "description": "Whether to crop vision matching to the selected window"},
				"search_x":          map[string]string{"type": "number", "description": "Optional search area X coordinate for image matching"},
				"search_y":          map[string]string{"type": "number", "description": "Optional search area Y coordinate for image matching"},
				"search_width":      map[string]string{"type": "number", "description": "Optional search area width for image matching"},
				"search_height":     map[string]string{"type": "number", "description": "Optional search area height for image matching"},
				"step":              map[string]string{"type": "number", "description": "Optional coarse image search step in pixels"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_activate_target", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopActivateTargetTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_set_target_value",
		"Set text into a resolved desktop target, preferring UI automation and falling back to click-and-type",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"strategy":          map[string]string{"type": "string", "description": "Optional single strategy: auto, window, ui, text, or image"},
				"strategies":        map[string]string{"type": "array", "description": "Optional ordered strategy list: window, ui, text, image"},
				"title":             map[string]string{"type": "string", "description": "Optional window title to match"},
				"process_name":      map[string]string{"type": "string", "description": "Optional process name to match, without .exe"},
				"handle":            map[string]string{"type": "number", "description": "Optional native window handle"},
				"match":             map[string]string{"type": "string", "description": "Optional title/name/class match mode: contains or exact"},
				"scope":             map[string]string{"type": "string", "description": "Optional UI automation scope: children or descendants"},
				"name":              map[string]string{"type": "string", "description": "Optional UI automation control name"},
				"automation_id":     map[string]string{"type": "string", "description": "Optional UI automation AutomationId"},
				"class_name":        map[string]string{"type": "string", "description": "Optional UI automation class name"},
				"control_type":      map[string]string{"type": "string", "description": "Optional UI automation control type"},
				"index":             map[string]string{"type": "number", "description": "Optional 1-based UI match index"},
				"include_offscreen": map[string]string{"type": "boolean", "description": "Whether to include offscreen UI controls"},
				"include_disabled":  map[string]string{"type": "boolean", "description": "Whether to include disabled UI controls"},
				"text":              map[string]string{"type": "string", "description": "Optional OCR text to find"},
				"mode":              map[string]string{"type": "string", "description": "Optional OCR match mode: contains, exact, or regex"},
				"ignore_case":       map[string]string{"type": "boolean", "description": "Whether OCR matching ignores case"},
				"occurrence":        map[string]string{"type": "number", "description": "Optional 1-based OCR match occurrence"},
				"min_confidence":    map[string]string{"type": "number", "description": "Optional minimum OCR confidence"},
				"lang":              map[string]string{"type": "string", "description": "Optional Tesseract language code"},
				"psm":               map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":               map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
				"template_path":     map[string]string{"type": "string", "description": "Optional template image path"},
				"threshold":         map[string]string{"type": "number", "description": "Optional image similarity threshold between 0 and 1"},
				"path":              map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"crop_to_window":    map[string]string{"type": "boolean", "description": "Whether to crop vision matching to the selected window"},
				"search_x":          map[string]string{"type": "number", "description": "Optional search area X coordinate for image matching"},
				"search_y":          map[string]string{"type": "number", "description": "Optional search area Y coordinate for image matching"},
				"search_width":      map[string]string{"type": "number", "description": "Optional search area width for image matching"},
				"search_height":     map[string]string{"type": "number", "description": "Optional search area height for image matching"},
				"step":              map[string]string{"type": "number", "description": "Optional coarse image search step in pixels"},
				"value":             map[string]string{"type": "string", "description": "Text value to enter"},
				"append":            map[string]string{"type": "boolean", "description": "Whether to append instead of replacing the current value"},
				"submit":            map[string]string{"type": "boolean", "description": "Whether to press Enter after entering the value"},
			},
			"required": []string{"value"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_set_target_value", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopSetTargetValueTool(ctx, input, opts)
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

	r.RegisterTool(
		"desktop_screenshot_window",
		"Capture a screenshot of a desktop window and save it to a file",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":         map[string]string{"type": "string", "description": "Destination PNG path inside the working directory"},
				"title":        map[string]string{"type": "string", "description": "Optional window title to match"},
				"process_name": map[string]string{"type": "string", "description": "Optional process name to match, without .exe"},
				"handle":       map[string]string{"type": "number", "description": "Optional native window handle"},
				"match":        map[string]string{"type": "string", "description": "Optional title match mode: contains or exact"},
				"active_only":  map[string]string{"type": "boolean", "description": "Whether to capture only a focused window match"},
			},
			"required": []string{"path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_screenshot_window", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopScreenshotWindowTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_match_image",
		"Find a template image on the desktop or in a screenshot",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":          map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"template_path": map[string]string{"type": "string", "description": "Template image path to locate"},
				"threshold":     map[string]string{"type": "number", "description": "Optional similarity threshold between 0 and 1"},
				"search_x":      map[string]string{"type": "number", "description": "Optional search area X coordinate"},
				"search_y":      map[string]string{"type": "number", "description": "Optional search area Y coordinate"},
				"search_width":  map[string]string{"type": "number", "description": "Optional search area width"},
				"search_height": map[string]string{"type": "number", "description": "Optional search area height"},
				"step":          map[string]string{"type": "number", "description": "Optional coarse search step in pixels"},
			},
			"required": []string{"template_path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_match_image", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopMatchImageTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_click_image",
		"Find a template image on the desktop and click its center",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":          map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"template_path": map[string]string{"type": "string", "description": "Template image path to locate"},
				"threshold":     map[string]string{"type": "number", "description": "Optional similarity threshold between 0 and 1"},
				"search_x":      map[string]string{"type": "number", "description": "Optional search area X coordinate"},
				"search_y":      map[string]string{"type": "number", "description": "Optional search area Y coordinate"},
				"search_width":  map[string]string{"type": "number", "description": "Optional search area width"},
				"search_height": map[string]string{"type": "number", "description": "Optional search area height"},
				"step":          map[string]string{"type": "number", "description": "Optional coarse search step in pixels"},
				"button":        map[string]string{"type": "string", "description": "Optional mouse button: left, right, middle"},
				"offset_x":      map[string]string{"type": "number", "description": "Optional X offset from the matched center"},
				"offset_y":      map[string]string{"type": "number", "description": "Optional Y offset from the matched center"},
				"double":        map[string]string{"type": "boolean", "description": "Optional double click toggle"},
				"interval_ms":   map[string]string{"type": "number", "description": "Optional double click interval in milliseconds"},
			},
			"required": []string{"template_path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_click_image", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopClickImageTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_wait_image",
		"Wait until a template image appears on the desktop",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"template_path": map[string]string{"type": "string", "description": "Template image path to locate"},
				"threshold":     map[string]string{"type": "number", "description": "Optional similarity threshold between 0 and 1"},
				"search_x":      map[string]string{"type": "number", "description": "Optional search area X coordinate"},
				"search_y":      map[string]string{"type": "number", "description": "Optional search area Y coordinate"},
				"search_width":  map[string]string{"type": "number", "description": "Optional search area width"},
				"search_height": map[string]string{"type": "number", "description": "Optional search area height"},
				"step":          map[string]string{"type": "number", "description": "Optional coarse search step in pixels"},
				"timeout_ms":    map[string]string{"type": "number", "description": "Optional timeout in milliseconds"},
				"interval_ms":   map[string]string{"type": "number", "description": "Optional poll interval in milliseconds"},
			},
			"required": []string{"template_path"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_wait_image", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopWaitImageTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_ocr",
		"Run OCR on the desktop or a screenshot image using the local OCR engine",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"lang": map[string]string{"type": "string", "description": "Optional Tesseract language code, e.g. eng or chi_sim"},
				"psm":  map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":  map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_ocr", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopOCRTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_verify_text",
		"Verify that OCR output contains expected text on the desktop",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":        map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"expected":    map[string]string{"type": "string", "description": "Expected OCR text"},
				"mode":        map[string]string{"type": "string", "description": "contains, exact, or regex"},
				"ignore_case": map[string]string{"type": "boolean", "description": "Whether to ignore case during matching"},
				"lang":        map[string]string{"type": "string", "description": "Optional Tesseract language code"},
				"psm":         map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":         map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
			},
			"required": []string{"expected"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_verify_text", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopVerifyTextTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_find_text",
		"Find visible text on the desktop via OCR and return its screen bounds",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":           map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"text":           map[string]string{"type": "string", "description": "Visible text to locate"},
				"mode":           map[string]string{"type": "string", "description": "contains, exact, or regex"},
				"ignore_case":    map[string]string{"type": "boolean", "description": "Whether to ignore case during matching"},
				"occurrence":     map[string]string{"type": "number", "description": "Optional 1-based match occurrence to return"},
				"min_confidence": map[string]string{"type": "number", "description": "Optional minimum OCR confidence threshold"},
				"lang":           map[string]string{"type": "string", "description": "Optional Tesseract language code"},
				"psm":            map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":            map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
			},
			"required": []string{"text"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_find_text", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopFindTextTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_click_text",
		"Find visible text on the desktop via OCR and click it",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":           map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"text":           map[string]string{"type": "string", "description": "Visible text to locate"},
				"mode":           map[string]string{"type": "string", "description": "contains, exact, or regex"},
				"ignore_case":    map[string]string{"type": "boolean", "description": "Whether to ignore case during matching"},
				"occurrence":     map[string]string{"type": "number", "description": "Optional 1-based match occurrence to return"},
				"min_confidence": map[string]string{"type": "number", "description": "Optional minimum OCR confidence threshold"},
				"lang":           map[string]string{"type": "string", "description": "Optional Tesseract language code"},
				"psm":            map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":            map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
				"button":         map[string]string{"type": "string", "description": "Optional mouse button: left, right, middle"},
				"offset_x":       map[string]string{"type": "number", "description": "Optional X offset from the matched center"},
				"offset_y":       map[string]string{"type": "number", "description": "Optional Y offset from the matched center"},
				"double":         map[string]string{"type": "boolean", "description": "Optional double click toggle"},
				"interval_ms":    map[string]string{"type": "number", "description": "Optional double click interval in milliseconds"},
			},
			"required": []string{"text"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_click_text", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopClickTextTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"desktop_wait_text",
		"Wait until visible text appears on the desktop via OCR",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":           map[string]string{"type": "string", "description": "Visible text to wait for"},
				"path":           map[string]string{"type": "string", "description": "Optional screenshot path; omit to capture the current desktop"},
				"mode":           map[string]string{"type": "string", "description": "contains, exact, or regex"},
				"ignore_case":    map[string]string{"type": "boolean", "description": "Whether to ignore case during matching"},
				"occurrence":     map[string]string{"type": "number", "description": "Optional 1-based match occurrence to return"},
				"min_confidence": map[string]string{"type": "number", "description": "Optional minimum OCR confidence threshold"},
				"timeout_ms":     map[string]string{"type": "number", "description": "Optional timeout in milliseconds"},
				"interval_ms":    map[string]string{"type": "number", "description": "Optional poll interval in milliseconds"},
				"lang":           map[string]string{"type": "string", "description": "Optional Tesseract language code"},
				"psm":            map[string]string{"type": "number", "description": "Optional Tesseract page segmentation mode"},
				"oem":            map[string]string{"type": "number", "description": "Optional Tesseract OCR engine mode"},
			},
			"required": []string{"text"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "desktop_wait_text", input, func(ctx context.Context, input map[string]any) (string, error) {
				return DesktopWaitTextTool(ctx, input, opts)
			})(ctx, input)
		},
	)

	r.RegisterTool(
		"image_analyze",
		"Analyze an image using a multimodal LLM (GPT-4V, Claude Vision, etc.). Describe objects, text, scenes, and visual content.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]string{"type": "string", "description": "Local image file path"},
				"url":    map[string]string{"type": "string", "description": "Image URL to analyze"},
				"prompt": map[string]string{"type": "string", "description": "Custom analysis prompt (optional)"},
			},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return auditCall(opts, "image_analyze", input, func(ctx context.Context, input map[string]any) (string, error) {
				return ImageAnalyzeTool(ctx, input, opts)
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
