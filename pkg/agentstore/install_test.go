package agentstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/plugin"
)

func TestStoreInstallProvisionsSkillPluginAndBindings(t *testing.T) {
	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, ".anyclaw")
	storeDir := filepath.Join(workDir, "store")
	sourcesDir := filepath.Join(workDir, "sources")
	skillSourceDir := filepath.Join(sourcesDir, "skill-bundle")
	appSourceDir := filepath.Join(sourcesDir, "app-bundle")

	for _, dir := range []string{storeDir, skillSourceDir, appSourceDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}

	if err := writeJSONFile(filepath.Join(skillSourceDir, "skill.json"), map[string]any{
		"name":        "demo-skill",
		"description": "Skill installed from store bundle",
		"version":     "1.0.0",
		"prompts": map[string]string{
			"system": "You are the demo skill.",
		},
	}); err != nil {
		t.Fatalf("write skill.json: %v", err)
	}

	appManifest := plugin.Manifest{
		Name:        "demo-app",
		Version:     "1.0.0",
		Description: "Demo app plugin",
		Kinds:       []string{"app"},
		Enabled:     true,
		Entrypoint:  "app.py",
		Permissions: []string{"tool:exec"},
		App: &plugin.AppSpec{
			Name:        "Demo App",
			Description: "Demo app plugin",
			Actions: []plugin.AppActionSpec{
				{Name: "ping", Description: "Ping the app", Kind: "execute"},
			},
			Workflows: []plugin.AppWorkflowSpec{
				{Name: "quick-ping", Description: "Quick ping", Action: "ping"},
			},
		},
	}
	if err := writeJSONFile(filepath.Join(appSourceDir, "plugin.json"), appManifest); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appSourceDir, "app.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write app.py: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Skills.Dir = filepath.Join(baseDir, "skills")
	cfg.Plugins.Dir = filepath.Join(baseDir, "plugins")
	cfg.Plugins.AllowExec = true
	cfg.Plugins.RequireTrust = true
	cfg.Plugins.TrustedSigners = []string{"dev-local"}
	cfg.Plugins.Enabled = []string{"existing-plugin"}
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:    "Go Expert",
			Enabled: config.BoolPtr(true),
			Skills: []config.AgentSkillRef{
				{Name: "coder", Enabled: true},
			},
		},
	}

	configPath := filepath.Join(baseDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	pkg := AgentPackage{
		ID:           "demo-package",
		Name:         "demo-package",
		DisplayName:  "Demo Package",
		Description:  "Demo package from store",
		SystemPrompt: "You help with the demo app.",
		Install: &InstallSpec{
			Skill: &SkillInstallSpec{
				Name: "demo-skill",
				Source: &InstallSource{
					LocalPath: "../sources/skill-bundle",
				},
			},
			App: &AppInstallSpec{
				Plugin: "demo-app-bundle",
				Source: &InstallSource{
					LocalPath: "../sources/app-bundle",
				},
				Signer: "dev-local",
			},
			Bindings: []*BindingInstallSpec{
				{
					Name:   "primary",
					Config: map[string]string{"token": "abc"},
				},
			},
			Pairings: []*PairingInstallSpec{
				{
					Name:     "quick-ping",
					Workflow: "quick-ping",
					Binding:  "primary",
					Triggers: []string{"ping demo"},
				},
			},
		},
	}
	if err := writeJSONFile(filepath.Join(storeDir, "demo-package.json"), pkg); err != nil {
		t.Fatalf("write package manifest: %v", err)
	}

	sm, err := NewStoreManager(workDir, configPath)
	if err != nil {
		t.Fatalf("NewStoreManager: %v", err)
	}
	if err := sm.Install("demo-package"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !sm.IsInstalled("demo-package") {
		t.Fatal("expected package to be marked installed")
	}

	skillPath := filepath.Join(cfg.Skills.Dir, "demo-skill", "skill.json")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected installed skill at %q: %v", skillPath, err)
	}

	loadedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}
	if !profileHasSkill(loadedCfg.Agent.Profiles, "Go Expert", "demo-skill") {
		t.Fatalf("expected Go Expert profile to include demo-skill: %#v", loadedCfg.Agent.Profiles)
	}
	if !containsFold(loadedCfg.Plugins.Enabled, "demo-app") {
		t.Fatalf("expected plugin enabled list to include demo-app: %#v", loadedCfg.Plugins.Enabled)
	}

	pluginDir := filepath.Join(cfg.Plugins.Dir, "demo-app")
	pluginManifestPath := filepath.Join(pluginDir, "plugin.json")
	manifestData, err := os.ReadFile(pluginManifestPath)
	if err != nil {
		t.Fatalf("read plugin manifest: %v", err)
	}
	var rawManifest map[string]any
	if err := json.Unmarshal(manifestData, &rawManifest); err != nil {
		t.Fatalf("unmarshal plugin manifest: %v", err)
	}
	if strings.TrimSpace(asString(rawManifest["signer"])) != "dev-local" {
		t.Fatalf("expected plugin signer dev-local, got %#v", rawManifest["signer"])
	}
	if strings.TrimSpace(asString(rawManifest["signature"])) == "" {
		t.Fatal("expected plugin signature to be written")
	}

	registry, err := plugin.NewRegistry(loadedCfg.Plugins)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if runners := registry.AppRunners(loadedCfg.Plugins.Dir); len(runners) != 1 {
		t.Fatalf("expected 1 app runner, got %d manifests=%#v", len(runners), registry.List())
	}

	appStore, err := apps.NewStore(configPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if bindings := appStore.ListByApp("demo-app"); len(bindings) != 1 {
		t.Fatalf("expected 1 app binding, got %d", len(bindings))
	}
	if pairings := appStore.ListPairingsByApp("demo-app"); len(pairings) != 1 {
		t.Fatalf("expected 1 app pairing, got %d", len(pairings))
	}

	if err := sm.Uninstall("demo-package"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if sm.IsInstalled("demo-package") {
		t.Fatal("expected package to be uninstalled")
	}
	if _, err := os.Stat(filepath.Join(cfg.Skills.Dir, "demo-skill")); !os.IsNotExist(err) {
		t.Fatalf("expected skill directory to be removed, got err=%v", err)
	}
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Fatalf("expected plugin directory to be removed, got err=%v", err)
	}

	loadedCfg, err = config.Load(configPath)
	if err != nil {
		t.Fatalf("Load config after uninstall: %v", err)
	}
	if profileHasSkill(loadedCfg.Agent.Profiles, "Go Expert", "demo-skill") {
		t.Fatalf("expected demo-skill to be removed from profile: %#v", loadedCfg.Agent.Profiles)
	}
	if containsFold(loadedCfg.Plugins.Enabled, "demo-app") {
		t.Fatalf("expected demo-app to be removed from enabled plugins: %#v", loadedCfg.Plugins.Enabled)
	}

	appStore, err = apps.NewStore(configPath)
	if err != nil {
		t.Fatalf("NewStore after uninstall: %v", err)
	}
	if bindings := appStore.ListByApp("demo-app"); len(bindings) != 0 {
		t.Fatalf("expected bindings to be removed, got %d", len(bindings))
	}
	if pairings := appStore.ListPairingsByApp("demo-app"); len(pairings) != 0 {
		t.Fatalf("expected pairings to be removed, got %d", len(pairings))
	}
}

func TestStoreInstallGeneratesSkillFromPackageMetadata(t *testing.T) {
	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, ".anyclaw")
	storeDir := filepath.Join(workDir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll store: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Skills.Dir = filepath.Join(baseDir, "skills")
	cfg.Plugins.Dir = filepath.Join(baseDir, "plugins")
	cfg.Agent.Profiles = []config.AgentProfile{
		{
			Name:    "Primary",
			Enabled: config.BoolPtr(true),
		},
	}
	configPath := filepath.Join(baseDir, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	pkg := AgentPackage{
		ID:           "generated-skill",
		Name:         "generated-skill",
		DisplayName:  "Generated Skill",
		Description:  "Generated from package metadata",
		SystemPrompt: "You are generated from metadata.",
	}
	if err := writeJSONFile(filepath.Join(storeDir, "generated-skill.json"), pkg); err != nil {
		t.Fatalf("write package manifest: %v", err)
	}

	sm, err := NewStoreManager(workDir, configPath)
	if err != nil {
		t.Fatalf("NewStoreManager: %v", err)
	}
	if err := sm.Install("generated-skill"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	skillPath := filepath.Join(cfg.Skills.Dir, "generated-skill", "skill.json")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read generated skill.json: %v", err)
	}
	var installed map[string]any
	if err := json.Unmarshal(data, &installed); err != nil {
		t.Fatalf("unmarshal generated skill.json: %v", err)
	}
	prompts, _ := installed["prompts"].(map[string]any)
	if strings.TrimSpace(asString(prompts["system"])) != "You are generated from metadata." {
		t.Fatalf("unexpected generated prompt: %#v", prompts)
	}

	loadedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}
	if !profileHasSkill(loadedCfg.Agent.Profiles, "Primary", "generated-skill") {
		t.Fatalf("expected generated-skill to be attached to profile: %#v", loadedCfg.Agent.Profiles)
	}
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func profileHasSkill(profiles []config.AgentProfile, profileName string, skillName string) bool {
	for _, profile := range profiles {
		if !strings.EqualFold(strings.TrimSpace(profile.Name), strings.TrimSpace(profileName)) {
			continue
		}
		for _, skill := range profile.Skills {
			if strings.EqualFold(strings.TrimSpace(skill.Name), strings.TrimSpace(skillName)) && skill.Enabled {
				return true
			}
		}
	}
	return false
}

func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
