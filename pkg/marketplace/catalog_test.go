package marketplace

import (
	"context"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/clihub"
	"github.com/1024XEngineer/anyclaw/pkg/config"
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
