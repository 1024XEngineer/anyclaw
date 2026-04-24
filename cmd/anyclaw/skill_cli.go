package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
)

type remoteSkillSearchItem struct {
	Name           string
	FullName       string
	Description    string
	Details        string
	InstallCommand string
}

type installedSkillRecord struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

var (
	searchRemoteSkills = skills.SearchSkills
	searchSkillCatalog = skills.SearchCatalog
	installGitHubSkill = skills.InstallSkillFromGitHub
)

func runSkillCommand(args []string) error {
	if len(args) == 0 {
		printSkillUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "search":
		return runSkillSearch(args[1:])
	case "install":
		return runSkillInstall(args[1:])
	case "list":
		return runSkillList(args[1:])
	case "info":
		return runSkillInfo(args[1:])
	case "catalog", "market", "registry":
		return runSkillCatalog(args[1:])
	default:
		printSkillUsage()
		return fmt.Errorf("unknown skill command: %s", args[0])
	}
}

func runSkillSearch(args []string) error {
	query := strings.TrimSpace(strings.Join(args, " "))
	fmt.Printf("Searching skills.sh: %s\n", query)

	results, err := searchRemoteSkills(context.Background(), query, 10)
	if err != nil {
		return fmt.Errorf("remote skill search failed: %w", err)
	}
	if len(results) == 0 {
		showBuiltinSkillsHelp()
		return nil
	}

	items := make([]remoteSkillSearchItem, 0, len(results))
	for _, result := range results {
		items = append(items, remoteSkillSearchItem{
			Name:           result.Name,
			FullName:       result.FullName,
			Description:    result.Description,
			Details:        fmt.Sprintf("installs: %s  stars: %d  %s", formatInstalls(result.Installs), result.Stars, getQualityBadge(result.Installs, result.Stars)),
			InstallCommand: "anyclaw skill install " + result.Name,
		})
	}
	printRemoteSkillResults(items)
	return nil
}

func runSkillInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw skill install <name>")
	}

	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("skill name is required")
	}

	skillsDir, err := ensureSkillsDir()
	if err != nil {
		return err
	}

	if content, ok := skills.BuiltinSkills[name]; ok {
		return installBuiltinSkill(name, content, skillsDir)
	}

	ctx := context.Background()
	if owner, repo, skillName, ok := parseSkillInstallRef(name); ok {
		if err := installGitHubSkill(ctx, owner, repo, skillName, skillsDir); err != nil {
			return err
		}
		printSuccess("Installed skill: %s", skillName)
		return nil
	}

	results, err := searchRemoteSkills(ctx, name, 1)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("skill not found: %s", name)
	}

	owner, repo, ok := resolveSkillSource(results[0].Source)
	if !ok {
		return fmt.Errorf("skill source is invalid: %s", results[0].Source)
	}
	if err := installGitHubSkill(ctx, owner, repo, results[0].Name, skillsDir); err != nil {
		return err
	}
	printSuccess("Installed skill: %s", results[0].Name)
	return nil
}

func runSkillList(args []string) error {
	items, err := readInstalledSkills(resolveSkillsDir())
	if err != nil {
		return err
	}
	if len(items) == 0 {
		printInfo("No local skills found.")
		return nil
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	for _, item := range items {
		fmt.Printf("- %s v%s\n", item.Name, firstNonEmptySkillField(item.Version, "1.0.0"))
		if desc := strings.TrimSpace(item.Description); desc != "" {
			fmt.Printf("  %s\n", desc)
		}
	}
	return nil
}

func runSkillInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw skill info <name>")
	}

	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("skill name is required")
	}

	manager := skills.NewSkillsManager(resolveSkillsDir())
	if err := manager.Load(); err != nil {
		return err
	}

	skill, ok := manager.Get(name)
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}

	fmt.Printf("Name: %s\n", skill.Name)
	fmt.Printf("Version: %s\n", firstNonEmptySkillField(skill.Version, "1.0.0"))
	fmt.Printf("Description: %s\n", skillDescription(skill.Description))
	if source := strings.TrimSpace(skill.Source); source != "" {
		fmt.Printf("Source: %s\n", source)
	}
	if registry := strings.TrimSpace(skill.Registry); registry != "" {
		fmt.Printf("Registry: %s\n", registry)
	}
	if entrypoint := strings.TrimSpace(skill.Entrypoint); entrypoint != "" {
		fmt.Printf("Entrypoint: %s\n", entrypoint)
	}
	if len(skill.Permissions) > 0 {
		fmt.Printf("Permissions: %s\n", strings.Join(skill.Permissions, ", "))
	}
	return nil
}

func runSkillCatalog(args []string) error {
	query := strings.TrimSpace(strings.Join(args, " "))
	entries, err := searchSkillCatalog(context.Background(), query, 20)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		printInfo("No skills found.")
		return nil
	}

	fmt.Println("Skill catalog:")
	for _, entry := range entries {
		fmt.Printf("- %s", skillDisplayName(entry.Name, entry.FullName))
		if version := strings.TrimSpace(entry.Version); version != "" {
			fmt.Printf(" v%s", version)
		}
		fmt.Println()
		fmt.Printf("  %s\n", skillDescription(entry.Description))
	}
	return nil
}

func printSkillUsage() {
	fmt.Print(`AnyClaw skill commands:

Usage:
  anyclaw skill search <query>
  anyclaw skill install <name>
  anyclaw skill install <owner>/<repo>/<skill>
  anyclaw skill list
  anyclaw skill info <name>
  anyclaw skill catalog [query]
`)
}

func readInstalledSkills(dir string) ([]installedSkillRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]installedSkillRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name(), "skill.json"))
		if err != nil {
			continue
		}

		var item installedSkillRecord
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if strings.TrimSpace(item.Name) == "" {
			item.Name = entry.Name()
		}
		items = append(items, item)
	}

	return items, nil
}

func installBuiltinSkill(name string, content string, skillsDir string) error {
	skillPath := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(skillPath, "skill.json"), []byte(content), 0o644); err != nil {
		return err
	}
	printSuccess("Installed skill: %s", name)
	return nil
}

func resolveSkillsDir() string {
	if skillsDir := strings.TrimSpace(os.Getenv("ANYCLAW_SKILLS_DIR")); skillsDir != "" {
		return skillsDir
	}
	return "skills"
}

func ensureSkillsDir() (string, error) {
	skillsDir := resolveSkillsDir()
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return "", err
	}
	return skillsDir, nil
}

func parseSkillInstallRef(name string) (string, string, string, bool) {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) != 3 {
		return "", "", "", false
	}
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if !isSafeSkillInstallSegment(part) {
			return "", "", "", false
		}
		parts[i] = part
	}
	return parts[0], parts[1], parts[2], true
}

func isSafeSkillInstallSegment(part string) bool {
	if part == "" || part == "." || part == ".." {
		return false
	}
	if strings.Contains(part, "/") || strings.Contains(part, "\\") {
		return false
	}
	if strings.Contains(part, ":") {
		return false
	}
	if filepath.IsAbs(part) || filepath.VolumeName(part) != "" {
		return false
	}
	return true
}

func resolveSkillSource(source string) (string, string, bool) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", false
	}

	source = strings.TrimPrefix(source, "github.com/")
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		parsed, err := url.Parse(source)
		if err != nil || !strings.Contains(strings.ToLower(parsed.Host), "github.com") {
			return "", "", false
		}
		source = strings.Trim(parsed.Path, "/")
	}

	parts := strings.Split(source, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func skillDisplayName(name string, fullName string) string {
	if fullName = strings.TrimSpace(fullName); fullName != "" {
		return fullName
	}
	return strings.TrimSpace(name)
}

func skillDescription(description string) string {
	if description = strings.TrimSpace(description); description != "" {
		return description
	}
	return "No description"
}

func formatInstalls(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return strconv.FormatInt(n, 10)
}

func getQualityBadge(installs int64, stars int) string {
	if installs >= 100000 || stars >= 1000 {
		return "premium"
	}
	if installs >= 10000 || stars >= 500 {
		return "popular"
	}
	if installs >= 1000 || stars >= 100 {
		return "recommended"
	}
	return "new"
}

func printRemoteSkillResults(items []remoteSkillSearchItem) {
	fmt.Printf("Found %d skills\n\n", len(items))
	for i, item := range items {
		fmt.Printf("%d. %s\n", i+1, skillDisplayName(item.Name, item.FullName))
		fmt.Printf("   %s\n", skillDescription(item.Description))
		if details := strings.TrimSpace(item.Details); details != "" {
			fmt.Printf("   %s\n", details)
		}
		fmt.Printf("   install: %s\n\n", item.InstallCommand)
	}
}

func showBuiltinSkillsHelp() {
	fmt.Println("No matching remote skills.")
	fmt.Println("Built-in skills:")

	names := make([]string, 0, len(skills.BuiltinSkills))
	for name := range skills.BuiltinSkills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("  - %s\n", name)
	}
}

func firstNonEmptySkillField(primary string, fallback string) string {
	if primary = strings.TrimSpace(primary); primary != "" {
		return primary
	}
	return strings.TrimSpace(fallback)
}
