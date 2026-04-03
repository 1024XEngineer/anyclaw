package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// PluginFormat represents the format of a plugin
type PluginFormat string

const (
	FormatJSON     PluginFormat = "json"
	FormatGo       PluginFormat = "go"
	FormatPython   PluginFormat = "python"
	FormatNode     PluginFormat = "node"
	FormatRust     PluginFormat = "rust"
	FormatBinary   PluginFormat = "binary"
	FormatWasm     PluginFormat = "wasm"
	FormatMCP      PluginFormat = "mcp"
	FormatOpenClaw PluginFormat = "openclaw"
	FormatClaude   PluginFormat = "claude"
	FormatCursor   PluginFormat = "cursor"
)

// PluginLoader loads plugins from various formats
type PluginLoader struct {
	mu       sync.RWMutex
	loaders  map[PluginFormat]FormatLoader
	baseDir  string
	cacheDir string
}

// FormatLoader is the interface for format-specific loaders
type FormatLoader interface {
	CanLoad(dir string, files []os.DirEntry) bool
	Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error)
	GetFormat() PluginFormat
}

// NewPluginLoader creates a new plugin loader
func NewPluginLoader(baseDir string) *PluginLoader {
	cacheDir := filepath.Join(baseDir, ".cache")
	os.MkdirAll(cacheDir, 0o755)

	loader := &PluginLoader{
		loaders:  make(map[PluginFormat]FormatLoader),
		baseDir:  baseDir,
		cacheDir: cacheDir,
	}

	// Register default loaders
	loader.RegisterLoader(&JSONLoader{})
	loader.RegisterLoader(&GoPluginLoader{})
	loader.RegisterLoader(&PythonPluginLoader{})
	loader.RegisterLoader(&NodePluginLoader{})
	loader.RegisterLoader(&RustPluginLoader{})
	loader.RegisterLoader(&BinaryPluginLoader{})
	loader.RegisterLoader(&MCPLoader{})
	loader.RegisterLoader(&OpenClawLoader{})
	loader.RegisterLoader(&ClaudeLoader{})

	return loader
}

// RegisterLoader registers a format loader
func (pl *PluginLoader) RegisterLoader(loader FormatLoader) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.loaders[loader.GetFormat()] = loader
}

// LoadPlugin loads a plugin from a directory
func (pl *PluginLoader) LoadPlugin(ctx context.Context, dir string, name string) (*Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	// Try each loader
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	for _, loader := range pl.loaders {
		if loader.CanLoad(dir, entries) {
			manifest, err := loader.Load(ctx, dir, name)
			if err == nil && manifest != nil {
				return manifest, nil
			}
		}
	}

	return nil, fmt.Errorf("no loader found for plugin in %s", dir)
}

// DiscoverPlugins discovers all plugins in a directory
func (pl *PluginLoader) DiscoverPlugins(ctx context.Context, pluginDir string) ([]*Manifest, error) {
	var manifests []*Manifest

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(pluginDir, entry.Name())
		manifest, err := pl.LoadPlugin(ctx, dir, entry.Name())
		if err != nil {
			continue
		}

		manifest.sourceDir = dir
		manifests = append(manifests, manifest)
	}

	return manifests, nil
}

// JSONLoader loads plugins from JSON manifests
type JSONLoader struct{}

func (l *JSONLoader) GetFormat() PluginFormat { return FormatJSON }

func (l *JSONLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		name := f.Name()
		if name == "plugin.json" || name == "openclaw.plugin.json" || name == "anyclaw.plugin.json" {
			return true
		}
	}
	return false
}

func (l *JSONLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	candidates := []string{
		"anyclaw.plugin.json",
		"plugin.json",
		"openclaw.plugin.json",
	}

	for _, name := range candidates {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		if manifest.Name == "" {
			manifest.Name = fallbackName
		}
		manifest.sourceDir = dir
		manifest.manifestPath = path
		return &manifest, nil
	}

	return nil, fmt.Errorf("no valid manifest found")
}

// GoPluginLoader loads Go plugins
type GoPluginLoader struct{}

func (l *GoPluginLoader) GetFormat() PluginFormat { return FormatGo }

func (l *GoPluginLoader) CanLoad(dir string, files []os.DirEntry) bool {
	hasGo := false
	hasManifest := false
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".go") {
			hasGo = true
		}
		if f.Name() == "plugin.json" || f.Name() == "go.mod" {
			hasManifest = true
		}
	}
	return hasGo && hasManifest
}

func (l *GoPluginLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	// Check for plugin.json first
	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// Create manifest from go.mod
		return l.createFromGoMod(dir, fallbackName)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	if manifest.Name == "" {
		manifest.Name = fallbackName
	}

	// Build if needed
	binaryPath := filepath.Join(dir, "plugin")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		if err := l.build(dir); err != nil {
			return nil, fmt.Errorf("failed to build Go plugin: %w", err)
		}
	}

	manifest.Entrypoint = binaryPath
	manifest.sourceDir = dir
	manifest.manifestPath = manifestPath
	return &manifest, nil
}

func (l *GoPluginLoader) createFromGoMod(dir string, fallbackName string) (*Manifest, error) {
	goModPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("no go.mod found")
	}

	// Extract module name
	lines := strings.Split(string(data), "\n")
	moduleName := fallbackName
	for _, line := range lines {
		if strings.HasPrefix(line, "module ") {
			moduleName = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			break
		}
	}

	return &Manifest{
		Name:        moduleName,
		Version:     "1.0.0",
		Description: "Go plugin",
		Kinds:       []string{"tool"},
		Enabled:     true,
		Entrypoint:  filepath.Join(dir, "plugin"),
		sourceDir:   dir,
	}, nil
}

func (l *GoPluginLoader) build(dir string) error {
	cmd := exec.Command("go", "build", "-o", "plugin", ".")
	cmd.Dir = dir
	return cmd.Run()
}

// PythonPluginLoader loads Python plugins
type PythonPluginLoader struct{}

func (l *PythonPluginLoader) GetFormat() PluginFormat { return FormatPython }

func (l *PythonPluginLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		name := f.Name()
		if name == "main.py" || name == "plugin.py" || name == "__main__.py" {
			return true
		}
		if name == "requirements.txt" || name == "pyproject.toml" {
			return true
		}
	}
	return false
}

func (l *PythonPluginLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	// Check for manifest
	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// Create from Python files
		return l.createFromPython(dir, fallbackName)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	if manifest.Name == "" {
		manifest.Name = fallbackName
	}

	// Find entry point
	entrypoint := l.findEntrypoint(dir)
	manifest.Entrypoint = entrypoint
	manifest.sourceDir = dir
	manifest.manifestPath = manifestPath
	return &manifest, nil
}

func (l *PythonPluginLoader) createFromPython(dir string, fallbackName string) (*Manifest, error) {
	entrypoint := l.findEntrypoint(dir)
	if entrypoint == "" {
		return nil, fmt.Errorf("no Python entry point found")
	}

	return &Manifest{
		Name:        fallbackName,
		Version:     "1.0.0",
		Description: "Python plugin",
		Kinds:       []string{"tool"},
		Enabled:     true,
		Entrypoint:  entrypoint,
		sourceDir:   dir,
	}, nil
}

func (l *PythonPluginLoader) findEntrypoint(dir string) string {
	candidates := []string{"main.py", "plugin.py", "__main__.py", "run.py"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// NodePluginLoader loads Node.js plugins
type NodePluginLoader struct{}

func (l *NodePluginLoader) GetFormat() PluginFormat { return FormatNode }

func (l *NodePluginLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		if f.Name() == "package.json" {
			return true
		}
	}
	return false
}

func (l *NodePluginLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	packageJSONPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, fmt.Errorf("no package.json found")
	}

	var pkg struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Main        string `json:"main"`
		Scripts     struct {
			Start string `json:"start"`
		} `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	name := pkg.Name
	if name == "" {
		name = fallbackName
	}

	entrypoint := pkg.Main
	if entrypoint == "" {
		entrypoint = "index.js"
	}

	// Check for plugin manifest
	manifestPath := filepath.Join(dir, "plugin.json")
	if manifestData, err := os.ReadFile(manifestPath); err == nil {
		var manifest Manifest
		if err := json.Unmarshal(manifestData, &manifest); err == nil {
			if manifest.Name == "" {
				manifest.Name = name
			}
			manifest.Entrypoint = filepath.Join(dir, entrypoint)
			manifest.sourceDir = dir
			manifest.manifestPath = manifestPath
			return &manifest, nil
		}
	}

	return &Manifest{
		Name:        name,
		Version:     pkg.Version,
		Description: pkg.Description,
		Kinds:       []string{"tool"},
		Enabled:     true,
		Entrypoint:  filepath.Join(dir, entrypoint),
		sourceDir:   dir,
	}, nil
}

// RustPluginLoader loads Rust plugins
type RustPluginLoader struct{}

func (l *RustPluginLoader) GetFormat() PluginFormat { return FormatRust }

func (l *RustPluginLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		if f.Name() == "Cargo.toml" {
			return true
		}
	}
	return false
}

func (l *RustPluginLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	cargoPath := filepath.Join(dir, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return nil, fmt.Errorf("no Cargo.toml found")
	}

	// Parse basic Cargo.toml
	name := fallbackName
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "name = ") {
			name = strings.Trim(strings.TrimPrefix(line, "name = "), "\"")
			break
		}
	}

	// Check for plugin manifest
	manifestPath := filepath.Join(dir, "plugin.json")
	if manifestData, err := os.ReadFile(manifestPath); err == nil {
		var manifest Manifest
		if err := json.Unmarshal(manifestData, &manifest); err == nil {
			if manifest.Name == "" {
				manifest.Name = name
			}
			manifest.sourceDir = dir
			manifest.manifestPath = manifestPath
			return &manifest, nil
		}
	}

	return &Manifest{
		Name:        name,
		Version:     "1.0.0",
		Description: "Rust plugin",
		Kinds:       []string{"tool"},
		Enabled:     true,
		sourceDir:   dir,
	}, nil
}

// BinaryPluginLoader loads pre-compiled binary plugins
type BinaryPluginLoader struct{}

func (l *BinaryPluginLoader) GetFormat() PluginFormat { return FormatBinary }

func (l *BinaryPluginLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".bin") || name == "plugin" {
			return true
		}
	}
	return false
}

func (l *BinaryPluginLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	// Find binary
	var binaryPath string
	entries, _ := os.ReadDir(dir)
	for _, f := range entries {
		name := f.Name()
		if name == "plugin" || strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".bin") {
			binaryPath = filepath.Join(dir, name)
			break
		}
	}

	if binaryPath == "" {
		return nil, fmt.Errorf("no binary found")
	}

	// Check for manifest
	manifestPath := filepath.Join(dir, "plugin.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err == nil {
			manifest.Entrypoint = binaryPath
			manifest.sourceDir = dir
			manifest.manifestPath = manifestPath
			return &manifest, nil
		}
	}

	return &Manifest{
		Name:        fallbackName,
		Version:     "1.0.0",
		Description: "Binary plugin",
		Kinds:       []string{"tool"},
		Enabled:     true,
		Entrypoint:  binaryPath,
		sourceDir:   dir,
	}, nil
}

// MCPLoader loads MCP (Model Context Protocol) plugins
type MCPLoader struct{}

func (l *MCPLoader) GetFormat() PluginFormat { return FormatMCP }

func (l *MCPLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		if f.Name() == "mcp.json" || f.Name() == ".mcp.json" {
			return true
		}
	}
	return false
}

func (l *MCPLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	candidates := []string{"mcp.json", ".mcp.json"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var mcpConfig struct {
			Name         string            `json:"name"`
			Version      string            `json:"version"`
			Description  string            `json:"description"`
			Command      string            `json:"command"`
			Args         []string          `json:"args"`
			Env          map[string]string `json:"env"`
			Transport    string            `json:"transport"`
			Capabilities []string          `json:"capabilities"`
		}
		if err := json.Unmarshal(data, &mcpConfig); err != nil {
			continue
		}

		name := mcpConfig.Name
		if name == "" {
			name = fallbackName
		}

		return &Manifest{
			Name:        name,
			Version:     mcpConfig.Version,
			Description: mcpConfig.Description,
			Kinds:       []string{"mcp", "tool"},
			Enabled:     true,
			MCP: &MCPSpec{
				Name:         name,
				Command:      mcpConfig.Command,
				Args:         mcpConfig.Args,
				Env:          mcpConfig.Env,
				Transport:    mcpConfig.Transport,
				Capabilities: mcpConfig.Capabilities,
			},
			sourceDir: dir,
		}, nil
	}

	return nil, fmt.Errorf("no MCP config found")
}

// OpenClawLoader loads OpenClaw-compatible plugins
type OpenClawLoader struct{}

func (l *OpenClawLoader) GetFormat() PluginFormat { return FormatOpenClaw }

func (l *OpenClawLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		if f.Name() == "openclaw.plugin.json" {
			return true
		}
	}
	return false
}

func (l *OpenClawLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	path := filepath.Join(dir, "openclaw.plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var openclawManifest map[string]any
	if err := json.Unmarshal(data, &openclawManifest); err != nil {
		return nil, err
	}

	// Convert OpenClaw manifest to AnyClaw manifest
	name, _ := openclawManifest["name"].(string)
	if name == "" {
		name = fallbackName
	}
	version, _ := openclawManifest["version"].(string)
	description, _ := openclawManifest["description"].(string)

	kinds := []string{"tool"}
	if k, ok := openclawManifest["kind"].(string); ok {
		kinds = []string{k}
	}

	manifest := &Manifest{
		Name:         name,
		Version:      version,
		Description:  description,
		Kinds:        kinds,
		Enabled:      true,
		sourceDir:    dir,
		manifestPath: path,
	}

	// Convert tool spec
	if tool, ok := openclawManifest["tool"].(map[string]any); ok {
		manifest.Tool = &ToolSpec{
			Name:        name,
			Description: description,
		}
		if schema, ok := tool["inputSchema"].(map[string]any); ok {
			manifest.Tool.InputSchema = schema
		}
	}

	// Convert channel spec
	if channel, ok := openclawManifest["channel"].(map[string]any); ok {
		manifest.Channel = &ChannelSpec{
			Name:        name,
			Description: description,
		}
		_ = channel
	}

	return manifest, nil
}

// ClaudeLoader loads Claude-compatible plugins
type ClaudeLoader struct{}

func (l *ClaudeLoader) GetFormat() PluginFormat { return FormatClaude }

func (l *ClaudeLoader) CanLoad(dir string, files []os.DirEntry) bool {
	for _, f := range files {
		if f.Name() == ".claude-plugin" {
			return true
		}
	}
	// Check for .claude-plugin directory
	for _, f := range files {
		if f.IsDir() && f.Name() == ".claude-plugin" {
			return true
		}
	}
	return false
}

func (l *ClaudeLoader) Load(ctx context.Context, dir string, fallbackName string) (*Manifest, error) {
	// Check for .claude-plugin/plugin.json
	manifestPath := filepath.Join(dir, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// Try root plugin.json
		manifestPath = filepath.Join(dir, "plugin.json")
		data, err = os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("no Claude plugin manifest found")
		}
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	if manifest.Name == "" {
		manifest.Name = fallbackName
	}
	manifest.sourceDir = dir
	manifest.manifestPath = manifestPath
	return &manifest, nil
}
