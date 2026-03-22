package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const SKILLSH_API = "https://skills.sh"

type SkillSearchResult struct {
	Name        string   `json:"name"`
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	Installs    int64    `json:"installs"`
	Stars       int      `json:"stars"`
	URL         string   `json:"url"`
	Version     string   `json:"version,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Entrypoint  string   `json:"entrypoint,omitempty"`
}

type SkillDetail struct {
	Name        string   `json:"name"`
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	Summary     string   `json:"summary"`
	Installs    int64    `json:"installs"`
	Stars       int      `json:"stars"`
	Repo        string   `json:"repo"`
	Markdown    string   `json:"markdown"`
	Version     string   `json:"version,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Entrypoint  string   `json:"entrypoint,omitempty"`
	Registry    string   `json:"registry,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
}

func SearchCatalog(ctx context.Context, query string, limit int) ([]SkillCatalogEntry, error) {
	results, err := SearchSkills(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]SkillCatalogEntry, 0, len(results))
	for _, r := range results {
		entries = append(entries, SkillCatalogEntry{
			Name:        r.Name,
			FullName:    r.FullName,
			Description: r.Description,
			Version:     r.Version,
			Registry:    "skills.sh",
			Homepage:    r.URL,
			Source:      r.URL,
			Permissions: append([]string(nil), r.Permissions...),
			Entrypoint:  r.Entrypoint,
			InstallHint: "anyclaw skill install " + r.Name,
		})
	}
	return entries, nil
}

func SearchSkills(ctx context.Context, query string, limit int) ([]SkillSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	searchURL := fmt.Sprintf("%s/api/search?q=%s&limit=%d", SKILLSH_API, url.QueryEscape(query), limit)

	client := &http.Client{Timeout: 30 * 1000000000}
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AnyClaw-SkillHub/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var results struct {
		Skills []SkillSearchResult `json:"skills"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, err
	}

	return results.Skills, nil
}

func GetSkillDetail(ctx context.Context, owner, repo, skillName string) (*SkillDetail, error) {
	detailURL := fmt.Sprintf("%s/api/skills/%s/%s/%s", SKILLSH_API, owner, repo, skillName)

	client := &http.Client{Timeout: 30 * 1000000000}
	req, err := http.NewRequestWithContext(ctx, "GET", detailURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AnyClaw-SkillHub/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skill not found: %s/%s", owner, repo)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var detail SkillDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

func GetSkillMarkdown(ctx context.Context, owner, repo, skillName string) (string, error) {
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/SKILL.md", owner, repo)

	client := &http.Client{Timeout: 30 * 1000000000}
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AnyClaw-SkillHub/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rawURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/master/SKILL.md", owner, repo)
		req, _ = http.NewRequestWithContext(ctx, "GET", rawURL, nil)
		req.Header.Set("User-Agent", "AnyClaw-SkillHub/1.0")
		resp, err = client.Do(req)
		if err != nil {
			return "", err
		}
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SKILL.md not found")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func InstallSkillFromGitHub(ctx context.Context, owner, repo, skillName string, destDir string) error {
	detail, _ := GetSkillDetail(ctx, owner, repo, skillName)
	md, err := GetSkillMarkdown(ctx, owner, repo, skillName)
	if err != nil {
		return fmt.Errorf("failed to fetch SKILL.md: %w", err)
	}

	skillJSON, err := ConvertMarkdownToSkillJSON(md, skillName, detail)
	if err != nil {
		return fmt.Errorf("failed to convert skill: %w", err)
	}

	skillDir := fmt.Sprintf("%s/%s", destDir, skillName)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(skillDir+"/skill.json", []byte(skillJSON), 0644)
}

func ConvertMarkdownToSkillJSON(md, name string, detail *SkillDetail) (string, error) {
	lines := strings.Split(md, "\n")

	var description string
	var systemPrompt strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "##") || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "- [") {
			continue
		}

		if strings.HasPrefix(line, "-") {
			systemPrompt.WriteString(strings.TrimPrefix(line, "- ") + " ")
			continue
		}

		if strings.HasPrefix(line, "**") && strings.HasSuffix(line, "**") {
			if description == "" {
				desc := strings.Trim(line, "*")
				description = desc
			}
			continue
		}

		if line == "" {
			continue
		}

		systemPrompt.WriteString(line + " ")
	}

	if systemPrompt.Len() == 0 {
		systemPrompt.WriteString("You are a helpful skill assistant.")
	}

	version := "1.0.0"
	permissions := []string{}
	entrypoint := ""
	homepage := ""
	registry := "skills.sh"
	if detail != nil {
		if strings.TrimSpace(detail.Version) != "" {
			version = strings.TrimSpace(detail.Version)
		}
		permissions = append(permissions, detail.Permissions...)
		entrypoint = strings.TrimSpace(detail.Entrypoint)
		homepage = strings.TrimSpace(detail.Homepage)
		if strings.TrimSpace(detail.Registry) != "" {
			registry = strings.TrimSpace(detail.Registry)
		}
	}
	permissionsJSON, _ := json.Marshal(permissions)
	result := fmt.Sprintf(`{
  "name": "%s",
  "description": "%s",
  "version": %q,
  "source": "skills.sh",
  "registry": %q,
  "homepage": %q,
  "entrypoint": %q,
  "permissions": %s,
  "install_command": %q,
  "prompts": {
    "system": %s
  }
}`, name, description, version, registry, homepage, entrypoint, string(permissionsJSON), "anyclaw skill install "+name, strconv.Quote(strings.TrimSpace(systemPrompt.String())))

	return result, nil
}
