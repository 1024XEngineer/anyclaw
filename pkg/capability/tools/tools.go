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
			return auditCall(opts, "web_search", input, func(ctx context.Context, input map[string]any) (string, error) {
				return WebSearchToolWithPolicy(ctx, input, opts)
			})(ctx, input)
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
			return auditCall(opts, "fetch_url", input, func(ctx context.Context, input map[string]any) (string, error) {
				return FetchURLToolWithPolicy(ctx, input, opts)
			})(ctx, input)
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
