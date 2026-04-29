package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

func TestMarketSourcesLoadsConfiguredSources(t *testing.T) {
	workDir := t.TempDir()
	if err := plugin.SaveSources(plugin.SourcesPath(workDir), []plugin.PluginSource{
		{Name: "internal", URL: "https://market.example.test", Type: "http"},
	}); err != nil {
		t.Fatalf("SaveSources: %v", err)
	}

	sources, err := marketSources(workDir)
	if err != nil {
		t.Fatalf("marketSources: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected default plus configured source, got %#v", sources)
	}
	if sources[0].Name != "default" {
		t.Fatalf("expected default source first, got %#v", sources)
	}
	if sources[1].Name != "internal" || sources[1].URL != "https://market.example.test" {
		t.Fatalf("expected configured source to be loaded, got %#v", sources)
	}
}

func TestMarketSourcesRejectsMalformedConfig(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "sources.json"), []byte(`not-json`), 0o644); err != nil {
		t.Fatalf("WriteFile sources.json: %v", err)
	}

	if _, err := marketSources(workDir); err == nil {
		t.Fatal("expected malformed sources config to return an error")
	}
}
