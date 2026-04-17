package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultSkillSearchLimit = 10
	remoteRegistryUserAgent = "AnyClaw-Skills/1.0"
)

type skillFileDefinition struct {
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Version        string            `json:"version"`
	Source         string            `json:"source,omitempty"`
	Registry       string            `json:"registry,omitempty"`
	Homepage       string            `json:"homepage,omitempty"`
	Entrypoint     string            `json:"entrypoint,omitempty"`
	Permissions    []string          `json:"permissions,omitempty"`
	InstallCommand string            `json:"install_command,omitempty"`
	Prompts        map[string]string `json:"prompts,omitempty"`
}

type catalogEntrySpec struct {
	Name         string
	FullName     string
	Description  string
	Version      string
	Category     string
	Registry     string
	Homepage     string
	Source       string
	Permissions  []string
	Entrypoint   string
	InstallHint  string
	Installed    bool
	InstalledDir string
	Builtin      bool
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultSkillSearchLimit
	}
	return limit
}

func newRemoteClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func newRemoteDownloadClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func doRemoteRequest(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", remoteRegistryUserAgent)
	return client.Do(req)
}

func fetchRemoteBody(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	resp, err := doRemoteRequest(ctx, client, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func fetchRemoteJSON(ctx context.Context, client *http.Client, rawURL string, target any) error {
	body, err := fetchRemoteBody(ctx, client, rawURL)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func fetchRemoteText(ctx context.Context, client *http.Client, rawURL string) (string, int, error) {
	resp, err := doRemoteRequest(ctx, client, rawURL)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}

func marshalSkillJSON(definition skillFileDefinition) ([]byte, error) {
	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeSkillJSONFile(skillDir string, data []byte) error {
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(skillDir, "skill.json"), data, 0o644)
}

func writeSkillFile(skillDir string, definition skillFileDefinition) error {
	data, err := marshalSkillJSON(definition)
	if err != nil {
		return err
	}
	return writeSkillJSONFile(skillDir, data)
}

func installSkillDefinition(destDir string, skillName string, definition skillFileDefinition) error {
	skillDir := filepath.Join(destDir, skillName)
	return writeSkillFile(skillDir, definition)
}

func buildCatalogEntries(specs []catalogEntrySpec) []SkillCatalogEntry {
	entries := make([]SkillCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, SkillCatalogEntry{
			Name:         spec.Name,
			FullName:     spec.FullName,
			Description:  spec.Description,
			Version:      spec.Version,
			Category:     spec.Category,
			Registry:     spec.Registry,
			Homepage:     spec.Homepage,
			Source:       spec.Source,
			Permissions:  append([]string(nil), spec.Permissions...),
			Entrypoint:   spec.Entrypoint,
			InstallHint:  spec.InstallHint,
			Installed:    spec.Installed,
			InstalledDir: spec.InstalledDir,
			Builtin:      spec.Builtin,
		})
	}
	return entries
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func pathWithinBase(baseDir, targetPath string) bool {
	baseDir = filepath.Clean(baseDir)
	targetPath = filepath.Clean(targetPath)
	if baseDir == targetPath {
		return true
	}
	return strings.HasPrefix(targetPath, baseDir+string(os.PathSeparator))
}
