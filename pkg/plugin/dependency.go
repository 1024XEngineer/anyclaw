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
	"time"
)

// Dependency represents a plugin dependency
type Dependency struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Source   string `json:"source,omitempty"` // npm:, pip:, cargo:, github:
	Optional bool   `json:"optional,omitempty"`
}

// DependencyManager manages plugin dependencies
type DependencyManager struct {
	mu        sync.RWMutex
	baseDir   string
	resolved  map[string]*Dependency
	installed map[string]string // name -> version
}

// NewDependencyManager creates a new dependency manager
func NewDependencyManager(baseDir string) *DependencyManager {
	dm := &DependencyManager{
		baseDir:   baseDir,
		resolved:  make(map[string]*Dependency),
		installed: make(map[string]string),
	}
	dm.loadInstalled()
	return dm
}

// loadInstalled loads already installed dependencies
func (dm *DependencyManager) loadInstalled() {
	installedDir := filepath.Join(dm.baseDir, "installed")
	entries, err := os.ReadDir(installedDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(installedDir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err == nil {
			dm.installed[manifest.Name] = manifest.Version
		}
	}
}

// Resolve resolves dependencies for a manifest
func (dm *DependencyManager) Resolve(ctx context.Context, manifest *Manifest) ([]Dependency, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var deps []Dependency

	// Check manifest dependencies
	if manifest.MCP != nil {
		deps = append(deps, Dependency{
			Name:   manifest.MCP.Name,
			Source: "mcp:" + manifest.MCP.Command,
		})
	}

	// Check for dependencies in manifest metadata
	if manifest.sourceDir != "" {
		depsFile := filepath.Join(manifest.sourceDir, "dependencies.json")
		if data, err := os.ReadFile(depsFile); err == nil {
			var manifestDeps []Dependency
			if err := json.Unmarshal(data, &manifestDeps); err == nil {
				deps = append(deps, manifestDeps...)
			}
		}
	}

	return deps, nil
}

// Install installs a dependency
func (dm *DependencyManager) Install(ctx context.Context, dep Dependency) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Check if already installed
	if version, ok := dm.installed[dep.Name]; ok {
		if version == dep.Version || dep.Version == "" {
			return nil // Already installed
		}
	}

	// Determine install method based on source
	if strings.HasPrefix(dep.Source, "npm:") {
		return dm.installNPM(ctx, dep)
	} else if strings.HasPrefix(dep.Source, "pip:") {
		return dm.installPip(ctx, dep)
	} else if strings.HasPrefix(dep.Source, "cargo:") {
		return dm.installCargo(ctx, dep)
	} else if strings.HasPrefix(dep.Source, "github:") {
		return dm.installGitHub(ctx, dep)
	} else if strings.HasPrefix(dep.Source, "mcp:") {
		return dm.installMCP(ctx, dep)
	}

	// Default: try to install from local path
	return dm.installLocal(ctx, dep)
}

// installNPM installs an npm package
func (dm *DependencyManager) installNPM(ctx context.Context, dep Dependency) error {
	packageDir := filepath.Join(dm.baseDir, "installed", dep.Name)
	os.MkdirAll(packageDir, 0o755)

	// Create package.json
	packageJSON := fmt.Sprintf(`{"name":"%s","version":"%s"}`, dep.Name, dep.Version)
	os.WriteFile(filepath.Join(packageDir, "package.json"), []byte(packageJSON), 0o644)

	// Run npm install
	cmd := exec.CommandContext(ctx, "npm", "install")
	cmd.Dir = packageDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm install failed: %w", err)
	}

	dm.installed[dep.Name] = dep.Version
	return nil
}

// installPip installs a Python package
func (dm *DependencyManager) installPip(ctx context.Context, dep Dependency) error {
	packageDir := filepath.Join(dm.baseDir, "installed", dep.Name)
	os.MkdirAll(packageDir, 0o755)

	// Create requirements.txt
	req := dep.Name
	if dep.Version != "" {
		req += "==" + dep.Version
	}
	os.WriteFile(filepath.Join(packageDir, "requirements.txt"), []byte(req), 0o644)

	// Run pip install
	cmd := exec.CommandContext(ctx, "pip", "install", "-r", "requirements.txt", "--target", packageDir)
	cmd.Dir = packageDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pip install failed: %w", err)
	}

	dm.installed[dep.Name] = dep.Version
	return nil
}

// installCargo installs a Rust crate
func (dm *DependencyManager) installCargo(ctx context.Context, dep Dependency) error {
	packageDir := filepath.Join(dm.baseDir, "installed", dep.Name)
	os.MkdirAll(packageDir, 0o755)

	// Create Cargo.toml
	cargoToml := fmt.Sprintf(`[package]
name = "%s"
version = "%s"

[dependencies]
%s = "%s"
`, dep.Name, "1.0.0", dep.Name, dep.Version)
	os.WriteFile(filepath.Join(packageDir, "Cargo.toml"), []byte(cargoToml), 0o644)

	// Run cargo build
	cmd := exec.CommandContext(ctx, "cargo", "build", "--release")
	cmd.Dir = packageDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cargo build failed: %w", err)
	}

	dm.installed[dep.Name] = dep.Version
	return nil
}

// installGitHub installs from a GitHub repository
func (dm *DependencyManager) installGitHub(ctx context.Context, dep Dependency) error {
	repo := strings.TrimPrefix(dep.Source, "github:")
	packageDir := filepath.Join(dm.baseDir, "installed", dep.Name)

	// Clone repository
	cmd := exec.CommandContext(ctx, "git", "clone", fmt.Sprintf("https://github.com/%s.git", repo), packageDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Checkout specific version if specified
	if dep.Version != "" && dep.Version != "latest" {
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", dep.Version)
		checkoutCmd.Dir = packageDir
		checkoutCmd.Run()
	}

	dm.installed[dep.Name] = dep.Version
	return nil
}

// installMCP installs an MCP server
func (dm *DependencyManager) installMCP(ctx context.Context, dep Dependency) error {
	command := strings.TrimPrefix(dep.Source, "mcp:")

	// Check if command exists
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("MCP command not found: %s", command)
	}

	dm.installed[dep.Name] = dep.Version
	return nil
}

// installLocal installs from a local path
func (dm *DependencyManager) installLocal(ctx context.Context, dep Dependency) error {
	// Check if source is a valid path
	if dep.Source != "" {
		if _, err := os.Stat(dep.Source); err == nil {
			packageDir := filepath.Join(dm.baseDir, "installed", dep.Name)
			os.MkdirAll(packageDir, 0o755)

			cmd := exec.CommandContext(ctx, "cp", "-r", dep.Source+"/.", packageDir)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("copy failed: %w", err)
			}

			dm.installed[dep.Name] = dep.Version
			return nil
		}
	}

	return fmt.Errorf("cannot install dependency: %s", dep.Name)
}

// Uninstall removes a dependency
func (dm *DependencyManager) Uninstall(name string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	packageDir := filepath.Join(dm.baseDir, "installed", name)
	if err := os.RemoveAll(packageDir); err != nil {
		return fmt.Errorf("failed to remove package: %w", err)
	}

	delete(dm.installed, name)
	return nil
}

// ListInstalled returns all installed dependencies
func (dm *DependencyManager) ListInstalled() map[string]string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range dm.installed {
		result[k] = v
	}
	return result
}

// CheckUpdates checks for available updates
func (dm *DependencyManager) CheckUpdates(ctx context.Context) (map[string]string, error) {
	updates := make(map[string]string)
	// This would typically query package registries
	// For now, return empty
	return updates, nil
}

// HotReload watches for plugin changes and reloads them
type HotReload struct {
	mu       sync.RWMutex
	watchDir string
	loader   *PluginLoader
	onChange func(manifest *Manifest)
	watcher  *FileWatcher
	stopCh   chan struct{}
}

// FileWatcher watches a directory for changes
type FileWatcher struct {
	dir      string
	interval time.Duration
	files    map[string]time.Time
	onChange func(path string)
	stopCh   chan struct{}
}

// NewHotReload creates a new hot-reload watcher
func NewHotReload(watchDir string, loader *PluginLoader, onChange func(manifest *Manifest)) *HotReload {
	return &HotReload{
		watchDir: watchDir,
		loader:   loader,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
}

// Start starts watching for changes
func (hr *HotReload) Start(ctx context.Context) error {
	hr.watcher = &FileWatcher{
		dir:      hr.watchDir,
		interval: 5 * time.Second,
		files:    make(map[string]time.Time),
		stopCh:   hr.stopCh,
		onChange: func(path string) {
			// Reload plugin
			dir := filepath.Dir(path)
			name := filepath.Base(dir)
			manifest, err := hr.loader.LoadPlugin(ctx, dir, name)
			if err == nil && hr.onChange != nil {
				hr.onChange(manifest)
			}
		},
	}

	go hr.watcher.Watch(ctx)
	return nil
}

// Stop stops watching
func (hr *HotReload) Stop() {
	close(hr.stopCh)
}

// Watch watches a directory for file changes
func (fw *FileWatcher) Watch(ctx context.Context) {
	ticker := time.NewTicker(fw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fw.stopCh:
			return
		case <-ticker.C:
			fw.scan()
		}
	}
}

// scan scans for changed files
func (fw *FileWatcher) scan() {
	filepath.Walk(fw.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		modTime := info.ModTime()
		if lastMod, ok := fw.files[path]; ok {
			if modTime.After(lastMod) {
				fw.files[path] = modTime
				if fw.onChange != nil {
					fw.onChange(path)
				}
			}
		} else {
			fw.files[path] = modTime
		}

		return nil
	})
}
