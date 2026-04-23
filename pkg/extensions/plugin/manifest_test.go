package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestNewRegistryPrefersOpenClawManifest(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "demo-surface")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	openclawManifest := Manifest{
		Name:        "demo-surface",
		Version:     "2.0.0",
		Description: "OpenClaw manifest",
		Kinds:       []string{"surface"},
		Enabled:     true,
		Entrypoint:  "surface.py",
		Surface: &SurfaceSpec{
			Name:        "demo-surface",
			Description: "OpenClaw surface",
			Path:        "/__openclaw__/surfaces/demo-surface",
		},
	}
	legacyManifest := Manifest{
		Name:        "demo-surface",
		Version:     "1.0.0",
		Description: "Legacy manifest",
		Kinds:       []string{"surface"},
		Enabled:     true,
		Entrypoint:  "legacy-surface.py",
		Surface: &SurfaceSpec{
			Name:        "demo-surface",
			Description: "Legacy surface",
			Path:        "/__anyclaw__/surfaces/demo-surface",
		},
	}
	if err := writeManifestFile(filepath.Join(pluginDir, "openclaw.plugin.json"), openclawManifest); err != nil {
		t.Fatalf("write openclaw manifest: %v", err)
	}
	if err := writeManifestFile(filepath.Join(pluginDir, "plugin.json"), legacyManifest); err != nil {
		t.Fatalf("write legacy manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "surface.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write entrypoint: %v", err)
	}

	registry, err := NewRegistry(config.PluginsConfig{Dir: baseDir, AllowExec: true})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	var loaded *Manifest
	for _, manifest := range registry.List() {
		if manifest.Name == "demo-surface" {
			loaded = &manifest
			break
		}
	}
	if loaded == nil {
		t.Fatal("expected demo-surface to be loaded")
	}
	if loaded.Version != "2.0.0" {
		t.Fatalf("expected openclaw manifest version, got %q", loaded.Version)
	}
	if loaded.Surface == nil || loaded.Surface.Path != "/__openclaw__/surfaces/demo-surface" {
		t.Fatalf("expected openclaw surface path, got %#v", loaded.Surface)
	}
}

func TestSurfaceRunnersResolveBundleManifestEntrypoint(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "bundle-surface")
	bundleDir := filepath.Join(pluginDir, ".codex-plugin")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := Manifest{
		Name:        "bundle-surface",
		Version:     "1.0.0",
		Description: "Bundle surface",
		Kinds:       []string{"surface"},
		Enabled:     true,
		Entrypoint:  "surface.py",
		Permissions: []string{"tool:exec"},
		Surface: &SurfaceSpec{
			Name:        "bundle-surface",
			Description: "Bundle surface",
			Path:        "/__openclaw__/surfaces/bundle-surface",
		},
	}
	if err := writeManifestFile(filepath.Join(bundleDir, "plugin.json"), manifest); err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}
	expectedEntrypoint := filepath.Join(bundleDir, "surface.py")
	if err := os.WriteFile(expectedEntrypoint, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write bundle entrypoint: %v", err)
	}

	registry, err := NewRegistry(config.PluginsConfig{
		Dir:          baseDir,
		AllowExec:    true,
		RequireTrust: false,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	runners := registry.SurfaceRunners(baseDir)
	if len(runners) != 1 {
		t.Fatalf("expected 1 surface runner, got %d", len(runners))
	}
	if got := runners[0].Entrypoint; got != expectedEntrypoint {
		t.Fatalf("expected bundle entrypoint %q, got %q", expectedEntrypoint, got)
	}
}

func TestRegistryPolicyBlocksPluginNetOutWithoutAllowedDomains(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "network-surface")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := Manifest{
		Name:        "network-surface",
		Version:     "1.0.0",
		Description: "Needs outbound network",
		Kinds:       []string{"surface"},
		Enabled:     true,
		Entrypoint:  "surface.py",
		Permissions: []string{"tool:exec", "net:out"},
		Surface: &SurfaceSpec{
			Name:        "network-surface",
			Description: "Network surface",
			Path:        "/__openclaw__/surfaces/network-surface",
		},
	}
	if err := writeManifestFile(filepath.Join(pluginDir, "plugin.json"), manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "surface.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write entrypoint: %v", err)
	}

	registry, err := NewRegistry(config.PluginsConfig{
		Dir:          baseDir,
		AllowExec:    true,
		RequireTrust: false,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registry.SetPolicyEngine(tools.NewPolicyEngine(tools.PolicyOptions{WorkingDir: t.TempDir()}))

	if runners := registry.SurfaceRunners(baseDir); len(runners) != 0 {
		t.Fatalf("expected policy to block net:out plugin, got %d runners", len(runners))
	}
}

func TestRegistryTrustAcceptsBareSHA256Signature(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "trusted-surface")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	entrypointPath := filepath.Join(pluginDir, "surface.py")
	if err := os.WriteFile(entrypointPath, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write entrypoint: %v", err)
	}

	manifest := Manifest{
		Name:        "trusted-surface",
		Version:     "1.0.0",
		Description: "Trusted surface",
		Kinds:       []string{"surface"},
		Enabled:     true,
		Entrypoint:  "surface.py",
		Permissions: []string{"tool:exec"},
		Signer:      "dev-local",
		Signature:   "ad64355106bb158b020ecf9702be48f7730fc091dd4bb6a2f092b40393495b3d",
		Surface: &SurfaceSpec{
			Name:        "Trusted Surface",
			Description: "Trusted surface",
			Path:        "/__openclaw__/surfaces/trusted-surface",
		},
	}
	if err := writeManifestFile(filepath.Join(pluginDir, "plugin.json"), manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	registry, err := NewRegistry(config.PluginsConfig{
		Dir:            baseDir,
		AllowExec:      true,
		RequireTrust:   true,
		TrustedSigners: []string{"dev-local"},
		Enabled:        []string{"trusted-surface"},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	foundTrustedSurface := false
	for _, item := range registry.List() {
		if item.Name == "trusted-surface" {
			foundTrustedSurface = true
			if !item.Verified {
				t.Fatalf("expected trusted-surface manifest to be verified, got %#v", item)
			}
		}
	}
	if !foundTrustedSurface {
		t.Fatalf("expected trusted-surface manifest to be loaded, got %#v", registry.List())
	}
}

func writeManifestFile(path string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
