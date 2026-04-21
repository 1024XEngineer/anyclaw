// Package extension provides the extension loading and management system.
// Extensions are self-contained modules that add channels, tools, providers,
// and other capabilities to AnyClaw, similar to OpenClaw's extensions/ architecture.
package extension

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manifest defines the extension metadata (anyclaw.extension.json).
type Manifest struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description"`
	Kind         string         `json:"kind"` // "channel", "tool", "provider", "memory", "hook"
	Builtin      bool           `json:"builtin,omitempty"`
	Channels     []string       `json:"channels,omitempty"`      // Channel IDs this extension provides
	Providers    []string       `json:"providers,omitempty"`     // LLM provider IDs
	Skills       []string       `json:"skills,omitempty"`        // Skill IDs bundled
	Entrypoint   string         `json:"entrypoint"`              // Main executable/script
	Permissions  []string       `json:"permissions,omitempty"`   // Required permissions
	ConfigSchema map[string]any `json:"config_schema,omitempty"` // JSON Schema for config
}

// Extension represents a loaded extension with runtime state.
type Extension struct {
	Manifest Manifest
	Path     string
	Enabled  bool
	Config   map[string]any
}

// Registry manages all loaded extensions.
type Registry struct {
	mu            sync.RWMutex
	extensions    map[string]*Extension
	extensionsDir string
}

// NewRegistry creates a new extension registry.
func NewRegistry(extensionsDir string) *Registry {
	return &Registry{
		extensions:    make(map[string]*Extension),
		extensionsDir: extensionsDir,
	}
}

// Discover scans the extensions directory for available extensions.
func (r *Registry) Discover() ([]Manifest, error) {
	manifests := append([]Manifest(nil), builtinExtensionManifests()...)

	entries, err := os.ReadDir(r.extensionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return manifests, nil
		}
		return nil, fmt.Errorf("failed to read extensions dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(r.extensionsDir, entry.Name(), "anyclaw.extension.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read manifest %s: %w", manifestPath, err)
		}

		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("invalid manifest %s: %w", manifestPath, err)
		}

		manifests = append(manifests, m)
	}

	return manifests, nil
}

// Register adds an extension to the registry.
func (r *Registry) Register(ext *Extension) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extensions[ext.Manifest.ID] = ext
}

// Get returns an extension by ID.
func (r *Registry) Get(id string) (*Extension, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ext, ok := r.extensions[id]
	return ext, ok
}

// List returns all registered extensions.
func (r *Registry) List() []*Extension {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Extension, 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext)
	}
	return result
}

// ListByKind returns extensions filtered by kind.
func (r *Registry) ListByKind(kind string) []*Extension {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*Extension
	for _, ext := range r.extensions {
		if ext.Manifest.Kind == kind && ext.Enabled {
			result = append(result, ext)
		}
	}
	return result
}

// Enable marks an extension as enabled.
func (r *Registry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ext, ok := r.extensions[id]
	if !ok {
		return fmt.Errorf("extension %q not found", id)
	}
	ext.Enabled = true
	return nil
}

// Disable marks an extension as disabled.
func (r *Registry) Disable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ext, ok := r.extensions[id]
	if !ok {
		return fmt.Errorf("extension %q not found", id)
	}
	ext.Enabled = false
	return nil
}

// LoadExtension loads a single extension from its directory.
func LoadExtension(dir string) (*Extension, error) {
	manifestPath := filepath.Join(dir, "anyclaw.extension.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	return &Extension{
		Manifest: m,
		Path:     dir,
		Enabled:  true,
	}, nil
}

// LoadAll discovers and loads all extensions from the registry's directory.
func (r *Registry) LoadAll() error {
	manifests, err := r.Discover()
	if err != nil {
		return err
	}

	for _, m := range manifests {
		if m.Builtin {
			r.Register(&Extension{
				Manifest: m,
				Path:     "builtin://extensions/" + m.ID,
				Enabled:  true,
			})
			continue
		}
		extPath := filepath.Join(r.extensionsDir, m.ID)
		ext, err := LoadExtension(extPath)
		if err != nil {
			return fmt.Errorf("failed to load extension %s: %w", m.ID, err)
		}
		r.Register(ext)
	}

	return nil
}
