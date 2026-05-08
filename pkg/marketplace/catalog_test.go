package marketplace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/clihub"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

func TestLocalCatalogListsCLIHubArtifacts(t *testing.T) {
	catalog := NewLocalCatalog(LocalCatalogDeps{
		CLIHub: &clihub.Catalog{
			Root: "C:/cli-anything",
			Entries: []clihub.Entry{
				{
					Name:        "drawio",
					DisplayName: "Draw.io",
					Version:     "1.2.3",
					Description: "Diagram automation",
					Category:    "diagram",
					EntryPoint:  "drawio",
					InstallCmd:  "https://example.test/install",
				},
			},
		},
	})

	result, err := catalog.List(context.Background(), Filter{Kind: ArtifactKindCLI, Source: SourceLocal})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 cli artifact, got %d: %#v", result.Total, result.Items)
	}
	item := result.Items[0]
	if item.ID != "cli:drawio" || item.Kind != ArtifactKindCLI {
		t.Fatalf("unexpected cli artifact identity: %#v", item)
	}
	if item.SourceID != "clihub" {
		t.Fatalf("expected clihub source id, got %q", item.SourceID)
	}
	if item.Status != StatusAvailable {
		t.Fatalf("expected unavailable catalog entry status to be available, got %q", item.Status)
	}
	if !containsString(item.Permissions, "process.exec") || !containsString(item.Permissions, "network.http") {
		t.Fatalf("expected cli permissions to include process.exec and network.http, got %#v", item.Permissions)
	}
}

func TestLocalCatalogGetAndVersions(t *testing.T) {
	catalog := NewLocalCatalog(LocalCatalogDeps{
		Config: &config.Config{
			Agent: config.AgentConfig{
				Name:            "main",
				Description:     "Main agent",
				PermissionLevel: "limited",
			},
		},
	})

	artifact, err := catalog.Get(context.Background(), "agent:main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if artifact.Status != StatusActive {
		t.Fatalf("expected active main agent, got %q", artifact.Status)
	}

	versions, err := catalog.Versions(context.Background(), "agent:main")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected no versions for unversioned local agent, got %#v", versions)
	}
}

func TestLocalCatalogListsProfilesSkillsPluginsAndFilters(t *testing.T) {
	baseDir := t.TempDir()
	skillsDir := filepath.Join(baseDir, "skills")
	pluginDir := filepath.Join(baseDir, "plugins")
	if err := os.MkdirAll(filepath.Join(skillsDir, "release-notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "release-notes", "skill.json"), []byte(`{
		"name":"release-notes",
		"full_name":"Release Notes",
		"description":"Writes release notes",
		"version":"1.2.0",
		"category":"writing",
		"permissions":["fs.read"],
		"source":"local"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "workflow", ".codex-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "workflow", ".codex-plugin", "plugin.json"), []byte(`{
		"name":"workflow",
		"version":"0.5.0",
		"description":"Workflow plugin",
		"kinds":["automation"],
		"enabled":true,
		"permissions":["network.http"],
		"capability_tags":["workflow"],
		"risk_level":"medium",
		"trust":"verified",
		"verified":true
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Agent.ActiveProfile = "Main"
	cfg.Agent.Profiles = []config.AgentProfile{
		{Name: "Main", Enabled: config.BoolPtr(true), Role: "coder", Domain: "engineering", Expertise: []string{"go"}, Skills: []config.AgentSkillRef{{Name: "release-notes"}}, PermissionLevel: "limited"},
		{Name: "Dormant", Enabled: config.BoolPtr(false), Role: "writer"},
	}
	cfg.Skills.Dir = skillsDir
	plugins, err := plugin.NewRegistry(config.PluginsConfig{Dir: pluginDir})
	if err != nil {
		t.Fatal(err)
	}
	catalog := NewLocalCatalog(LocalCatalogDeps{
		Config:  cfg,
		Skills:  loadedSkillsManager(t, skillsDir),
		Plugins: plugins,
	})

	result, err := catalog.List(context.Background(), Filter{Source: SourceLocal, Limit: 2, Offset: -10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Limit != 2 || result.Offset != 0 || len(result.Items) != 2 || result.Total < 4 {
		t.Fatalf("unexpected paged result: %#v", result)
	}
	skill, err := catalog.Get(context.Background(), "skill:release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if skill.Status != StatusInstalled || skill.SourceID != "local" || !containsString(skill.Capabilities, "writing") {
		t.Fatalf("unexpected skill artifact: %#v", skill)
	}
	versions, err := catalog.Versions(context.Background(), "skill:release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Version != "1.2.0" {
		t.Fatalf("unexpected versions: %#v", versions)
	}
	filtered, err := catalog.List(context.Background(), Filter{Kind: ArtifactKindPlugin, Query: "workflow", Risk: "medium", Trust: "verified", Permission: "network.http", Tag: "automation", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || filtered.Items[0].ID != "plugin:workflow" || !filtered.Items[0].Verified {
		t.Fatalf("unexpected filtered plugin: %#v", filtered)
	}
	empty, err := catalog.List(context.Background(), Filter{Source: SourceCloud})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Total != 0 {
		t.Fatalf("expected no local results for cloud filter, got %#v", empty)
	}
	if _, err := catalog.Get(context.Background(), ""); err != ErrArtifactNotFound {
		t.Fatalf("expected ErrArtifactNotFound for empty id, got %v", err)
	}
}

func loadedSkillsManager(t *testing.T, dir string) *skills.SkillsManager {
	t.Helper()
	manager := skills.NewSkillsManager(dir)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestCatalogHelpers(t *testing.T) {
	items := []Artifact{
		{ID: "", Source: SourceLocal},
		{ID: "skill:a", Name: "A", Source: SourceLocal, SourceID: "skills"},
		{ID: "skill:a", Name: "A duplicate", Source: SourceLocal, SourceID: "skills"},
		{ID: "skill:a", Name: "A cloud", Source: SourceCloud, SourceID: "cloud"},
	}
	deduped := dedupeArtifacts(items)
	if len(deduped) != 2 {
		t.Fatalf("deduped = %#v", deduped)
	}
	if !containsFold([]string{" Alpha "}, "alpha") || !containsFold([]string{"Alpha"}, "") {
		t.Fatal("containsFold did not match expected values")
	}
	if got := appendUnique([]string{"A"}, "a", "B", " "); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("appendUnique = %#v", got)
	}
	if firstNonEmpty(" ", "x") != "x" || firstString([]string{"", "y"}) != "y" || boolString(true) != "true" || boolString(false) != "false" {
		t.Fatal("basic helper output mismatch")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
