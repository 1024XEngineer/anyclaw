package plugin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const sourcesFileName = "sources.json"

// DefaultSources returns the built-in plugin market sources.
func DefaultSources() []PluginSource {
	return []PluginSource{
		{Name: "default", URL: "https://market.anyclaw.github.io", Type: "http"},
	}
}

// SourcesPath returns the market source config path under the runtime work dir.
func SourcesPath(workDir string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = ".anyclaw"
	}
	return filepath.Join(workDir, sourcesFileName)
}

func LoadSources(path string) ([]PluginSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sources []PluginSource
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("parse sources: %w", err)
	}
	return NormalizeSources(sources)
}

func SaveSources(path string, sources []PluginSource) error {
	normalized, err := NormalizeSources(sources)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func NormalizeSources(sources []PluginSource) ([]PluginSource, error) {
	normalized := make([]PluginSource, 0, len(sources))
	seen := map[string]struct{}{}
	for _, source := range sources {
		item, err := NormalizeSource(source)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(item.Name)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate source name: %s", item.Name)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func NormalizeSource(source PluginSource) (PluginSource, error) {
	source.Name = strings.TrimSpace(source.Name)
	source.URL = strings.TrimSpace(source.URL)
	source.Type = strings.ToLower(strings.TrimSpace(source.Type))
	source.Auth = strings.TrimSpace(source.Auth)
	source.Branch = strings.TrimSpace(source.Branch)

	if source.Type == "" {
		source.Type = "http"
	}
	if source.Name == "" {
		return PluginSource{}, fmt.Errorf("source name is required")
	}
	if source.URL == "" {
		return PluginSource{}, fmt.Errorf("source url is required")
	}
	switch source.Type {
	case "http", "github":
		parsed, err := url.ParseRequestURI(source.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return PluginSource{}, fmt.Errorf("source %s has invalid url: %s", source.Name, source.URL)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return PluginSource{}, fmt.Errorf("source %s must use http or https", source.Name)
		}
	default:
		return PluginSource{}, fmt.Errorf("unsupported source type %q for %s", source.Type, source.Name)
	}
	return source, nil
}

func MergeSources(base []PluginSource, extra []PluginSource) []PluginSource {
	merged := make([]PluginSource, 0, len(base)+len(extra))
	indexByName := map[string]int{}
	for _, source := range append(base, extra...) {
		item, err := NormalizeSource(source)
		if err != nil {
			continue
		}
		key := strings.ToLower(item.Name)
		if idx, ok := indexByName[key]; ok {
			merged[idx] = item
			continue
		}
		indexByName[key] = len(merged)
		merged = append(merged, item)
	}
	return merged
}
