package skills

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const SKILLHUB_SEARCH_URL = "https://lightmake.site/api/v1/search"
const SKILLHUB_DOWNLOAD_URL = "https://lightmake.site/api/v1/download"

type SkillhubSearchResult struct {
	Name        string   `json:"name"`
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	Downloads   int64    `json:"downloads"`
	Stars       int      `json:"stars"`
	URL         string   `json:"url"`
	Version     string   `json:"version,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func SearchSkillhub(ctx context.Context, query string, limit int) ([]SkillhubSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	searchURL := fmt.Sprintf("%s?q=%s&limit=%d", SKILLHUB_SEARCH_URL, url.QueryEscape(query), limit)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AnyClaw-Skillhub/1.0")

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

	var apiResponse struct {
		Results []struct {
			DisplayName string  `json:"displayName"`
			Score       float64 `json:"score"`
			Slug        string  `json:"slug"`
			Summary     string  `json:"summary"`
			UpdatedAt   int64   `json:"updatedAt"`
			Version     string  `json:"version"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, err
	}

	results := make([]SkillhubSearchResult, 0, len(apiResponse.Results))
	for _, r := range apiResponse.Results {
		results = append(results, SkillhubSearchResult{
			Name:        r.Slug,
			FullName:    r.DisplayName,
			Description: r.Summary,
			Version:     r.Version,
		})
	}

	return results, nil
}

func InstallSkillhubSkill(ctx context.Context, skillName string, destDir string) error {
	downloadURL := fmt.Sprintf("%s?slug=%s", SKILLHUB_DOWNLOAD_URL, url.QueryEscape(skillName))

	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "AnyClaw-Skillhub/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	skillDir := filepath.Join(destDir, skillName)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "skillhub-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("failed to save download: %w", err)
	}
	tmpFile.Close()

	if err := extractZip(tmpFile.Name(), skillDir); err != nil {
		return fmt.Errorf("failed to extract skill: %w", err)
	}

	if err := ConvertSkillhubToSkillJSON(skillDir); err != nil {
		return fmt.Errorf("failed to convert skill: %w", err)
	}

	return nil
}

func extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(destDir, file.Name)

		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destDir)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

func ConvertSkillhubToSkillJSON(skillDir string) error {
	skillMdPath := filepath.Join(skillDir, "SKILL.md")
	skillJSONPath := filepath.Join(skillDir, "skill.json")

	if _, err := os.Stat(skillJSONPath); err == nil {
		return nil
	}

	if _, err := os.Stat(skillMdPath); err != nil {
		return fmt.Errorf("SKILL.md not found")
	}

	content, err := os.ReadFile(skillMdPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var name, description, systemPrompt strings.Builder
	inFrontmatter := false
	frontmatterDone := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				frontmatterDone = true
				continue
			}
		}

		if inFrontmatter && !frontmatterDone {
			if strings.HasPrefix(line, "name:") {
				name.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "name:")))
			} else if strings.HasPrefix(line, "description:") {
				description.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "description:")))
			}
			continue
		}

		if frontmatterDone && line != "" {
			systemPrompt.WriteString(line + "\n")
		}
	}

	if name.Len() == 0 {
		name.WriteString(filepath.Base(skillDir))
	}
	if description.Len() == 0 {
		description.WriteString("Skill from Skillhub")
	}
	if systemPrompt.Len() == 0 {
		systemPrompt.WriteString("You are a helpful assistant.")
	}

	skillJSON := fmt.Sprintf(`{
  "name": %q,
  "description": %q,
  "version": "1.0.0",
  "source": "skillhub",
  "prompts": {
    "system": %q
  }
}`, name.String(), description.String(), systemPrompt.String())

	return os.WriteFile(skillJSONPath, []byte(skillJSON), 0644)
}

func IsSkillhubInstalled() bool {
	return true
}

func FindSkillhubCLIPath() (string, error) {
	return "integrated", nil
}

func SearchSkillhubCatalog(ctx context.Context, query string, limit int) ([]SkillCatalogEntry, error) {
	results, err := SearchSkillhub(ctx, query, limit)
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
			Category:    r.Category,
			Registry:    "skillhub",
			Homepage:    r.URL,
			Source:      r.URL,
			InstallHint: "anyclaw skillhub install " + r.Name,
		})
	}

	return entries, nil
}
