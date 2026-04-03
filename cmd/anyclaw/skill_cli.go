package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/consoleio"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/ui"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var titleCase = cases.Title(language.English)

type remoteSkillSearchItem struct {
	Name           string
	FullName       string
	Description    string
	Category       string
	Details        string
	InstallCommand string
}

func runSkillCommand() {
	if len(os.Args) < 3 {
		printSkillUsage()
		return
	}

	args := os.Args[2:]
	switch args[0] {
	case "search":
		query := ""
		if len(args) > 1 {
			query = strings.Join(args[1:], " ")
		}
		searchSkillsFromHub(query)
	case "install":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: anyclaw skill install <name>")
			os.Exit(1)
		}
		installSkillFromHub(args[1])
	case "list":
		listInstalledSkills()
	case "info":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: anyclaw skill info <name>")
			os.Exit(1)
		}
		showSkillInfo(args[1])
	case "catalog", "market", "registry":
		query := ""
		if len(args) > 1 {
			query = strings.Join(args[1:], " ")
		}
		showSkillCatalog(query)
	case "create":
		createNewSkill()
	default:
		fmt.Fprintf(os.Stderr, "unknown skill command: %s\n", args[0])
		printSkillUsage()
		os.Exit(1)
	}
}

func runSkillhubCommand() {
	runHubRegistryCommand("skillhub")
}

func runClawhubCommand() {
	runHubRegistryCommand("clawhub")
}

func runHubRegistryCommand(commandName string) {
	if len(os.Args) < 3 {
		printSkillhubUsage(commandName)
		return
	}

	args := os.Args[2:]
	switch args[0] {
	case "search":
		query := ""
		if len(args) > 1 {
			query = strings.Join(args[1:], " ")
		}
		searchSkillhubFromCLI(query, commandName)
	case "install":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: anyclaw %s install <name>\n", commandName)
			os.Exit(1)
		}
		installSkillhubFromCLI(args[1], commandName)
	case "list":
		listSkillhubSkills(commandName)
	case "check":
		checkSkillhubCLI(commandName)
	case "update":
		target := ""
		if len(args) > 1 {
			target = strings.TrimSpace(args[1])
		}
		updateSkillhubSkills(target, commandName)
	default:
		fmt.Fprintf(os.Stderr, "unknown %s command: %s\n", commandName, args[0])
		printSkillhubUsage(commandName)
		os.Exit(1)
	}
}

func printSkillUsage() {
	fmt.Print(`AnyClaw skill commands:

Usage:
  anyclaw skill search <query>
  anyclaw skill install <name>
  anyclaw skill list
  anyclaw skill info <name>
  anyclaw skill catalog [query]
  anyclaw skill create
`)
}

func printSkillhubUsage(commandName string) {
	fmt.Printf(`AnyClaw %s commands:

Usage:
  anyclaw %s search <query>
  anyclaw %s install <name>
  anyclaw %s list
  anyclaw %s check
  anyclaw %s update [name]
`, commandName, commandName, commandName, commandName, commandName, commandName)
}

func searchSkillhubFromCLI(query string, commandName string) {
	fmt.Printf("Searching %s: %s\n", titleCase.String(commandName), query)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))

	ctx := context.Background()
	results, err := skills.SearchSkillhub(ctx, query, 10)
	if err != nil {
		printError("search failed: %v", err)
		return
	}
	if len(results) == 0 {
		printInfo("No skills found.")
		return
	}

	items := make([]remoteSkillSearchItem, 0, len(results))
	for _, r := range results {
		items = append(items, remoteSkillSearchItem{
			Name:           r.Name,
			FullName:       r.FullName,
			Description:    r.Description,
			Category:       r.Category,
			InstallCommand: fmt.Sprintf("anyclaw %s install %s", commandName, r.Name),
		})
	}
	printRemoteSkillResults(items)
}

func installSkillhubFromCLI(skillName string, commandName string) {
	fmt.Printf("Installing %s skill: %s\n", commandName, skillName)
	ctx := context.Background()
	skillsDir, err := ensureSkillsDir()
	if err != nil {
		printError("failed to create skills dir: %v", err)
		return
	}
	if err := skills.InstallSkillhubSkill(ctx, skillName, skillsDir); err != nil {
		printError("install failed: %v", err)
		return
	}
	printSuccess("Installed %s skill: %s", commandName, skillName)
}

func listSkillhubSkills(commandName string) {
	skillsDir := resolveSkillsDir()
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		printInfo("No installed skills.")
		return
	}
	var list []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillsDir, entry.Name(), "skill.json")); err == nil {
			list = append(list, entry.Name())
		}
	}
	if len(list) == 0 {
		printInfo("No installed skills.")
		return
	}
	fmt.Printf("%s\n\n", ui.Bold.Sprint(titleCase.String(commandName)+" skills"))
	for _, name := range list {
		fmt.Printf("  - %s\n", name)
	}
}

func checkSkillhubCLI(commandName string) {
	printSuccess("%s CLI is available", titleCase.String(commandName))
	printInfo("Use `anyclaw %s search <query>` to search", commandName)
}

func updateSkillhubSkills(target string, commandName string) {
	manager := skills.NewSkillsManager(resolveSkillsDir())
	if err := manager.Load(); err != nil {
		printInfo("No installed skills to update.")
		return
	}
	selected := make([]string, 0)
	if target != "" {
		skill, ok := manager.Get(target)
		if !ok {
			printError("skill not found: %s", target)
			return
		}
		if !isHubInstalledSkill(skill) {
			printError("skill %s is not installed from %s", target, commandName)
			return
		}
		selected = append(selected, skill.Name)
	} else {
		for _, skill := range manager.List() {
			if isHubInstalledSkill(skill) {
				selected = append(selected, skill.Name)
			}
		}
	}
	if len(selected) == 0 {
		printInfo("No %s-installed skills to update.", commandName)
		return
	}
	ctx := context.Background()
	skillsDir := resolveSkillsDir()
	for _, name := range selected {
		if err := skills.InstallSkillhubSkill(ctx, name, skillsDir); err != nil {
			printError("update failed for %s: %v", name, err)
			return
		}
	}
	printSuccess("Updated %d %s skill(s)", len(selected), commandName)
}

func isHubInstalledSkill(skill *skills.Skill) bool {
	if skill == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(skill.Source), "skillhub") || strings.EqualFold(strings.TrimSpace(skill.Registry), "skillhub")
}

func searchSkillsFromHub(query string) {
	fmt.Printf("Searching skills.sh: %s\n", query)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))

	ctx := context.Background()
	results, err := skills.SearchSkills(ctx, query, 10)
	if err != nil || len(results) == 0 {
		showBuiltinSkillsHelp()
		return
	}

	items := make([]remoteSkillSearchItem, 0, len(results))
	for _, r := range results {
		installs := formatInstalls(r.Installs)
		items = append(items, remoteSkillSearchItem{
			Name:           r.Name,
			FullName:       r.FullName,
			Description:    r.Description,
			Details:        fmt.Sprintf("installs: %s  stars: %d  %s", installs, r.Stars, getQualityBadge(r.Installs, r.Stars)),
			InstallCommand: "anyclaw skill install " + r.Name,
		})
	}
	printRemoteSkillResults(items)
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

func showBuiltinSkillsHelp() {
	fmt.Println("No matching remote skills.")
	fmt.Println("Built-in skills:")
	for name := range skills.BuiltinSkills {
		fmt.Printf("  - %s\n", name)
	}
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

func installSkillFromHub(name string) {
	skillsDir := resolveSkillsDir()

	if content, ok := skills.BuiltinSkills[name]; ok {
		installBuiltinSkill(name, content, skillsDir)
		return
	}

	parts := strings.Split(name, "/")
	if len(parts) == 3 {
		ctx := context.Background()
		if err := skills.InstallSkillFromGitHub(ctx, parts[0], parts[1], parts[2], skillsDir); err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			os.Exit(1)
		}
		printSuccess("Installed: %s", name)
		return
	}

	// Search skills.sh for the skill
	ctx := context.Background()
	results, err := skills.SearchSkills(ctx, name, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search error: %v\n", err)
	} else if len(results) > 0 {
		skill := results[0]
		// Use Source field which contains owner/repo format
		if skill.Source != "" {
			sourceParts := strings.Split(skill.Source, "/")
			if len(sourceParts) == 2 {
				// Source is owner/repo, need to get skill name from URL or Name
				skillName := skill.Name
				if err := skills.InstallSkillFromGitHub(ctx, sourceParts[0], sourceParts[1], skillName, skillsDir); err != nil {
					fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
					os.Exit(1)
				}
				printSuccess("Installed: %s", skill.Name)
				return
			}
		}
	}

	fmt.Fprintf(os.Stderr, "skill not found: %s\n", name)
	os.Exit(1)
}

func installBuiltinSkill(name, content, skillsDir string) {
	skillPath := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create skill dir: %v\n", err)
		os.Exit(1)
	}
	filePath := filepath.Join(skillPath, "skill.json")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write skill: %v\n", err)
		os.Exit(1)
	}
	printSuccess("Installed skill: %s", name)
}

func listInstalledSkills() {
	manager := skills.NewSkillsManager(resolveSkillsDir())
	if err := manager.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load skills: %v\n", err)
		os.Exit(1)
	}
	list := manager.List()
	if len(list) == 0 {
		fmt.Println("No skills installed.")
		return
	}
	for _, s := range list {
		fmt.Printf("- %s v%s\n", s.Name, s.Version)
		fmt.Printf("  %s\n", s.Description)
	}
}

func showSkillInfo(name string) {
	manager := skills.NewSkillsManager(resolveSkillsDir())
	_ = manager.Load()
	if skill, ok := manager.Get(name); ok {
		fmt.Printf("Name: %s\nVersion: %s\nDescription: %s\n", skill.Name, skill.Version, skill.Description)
		fmt.Printf("Source: %s\nRegistry: %s\nEntrypoint: %s\n", skill.Source, skill.Registry, skill.Entrypoint)
		return
	}
	fmt.Fprintf(os.Stderr, "skill not found: %s\n", name)
	os.Exit(1)
}

func showSkillCatalog(query string) {
	ctx := context.Background()
	entries, err := skills.SearchCatalog(ctx, query, 20)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog load failed: %v\n", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Println("No skills found.")
		return
	}
	fmt.Println("Skill catalog:")
	for _, entry := range entries {
		fmt.Printf("- %s v%s\n", skillDisplayName(entry.Name, entry.FullName), entry.Version)
		fmt.Printf("  %s\n", skillDescription(entry.Description))
	}
}

func createNewSkill() {
	reader := consoleio.NewReader(os.Stdin)
	fmt.Print("Skill name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		printError("skill name is required")
		return
	}

	fmt.Print("Description: ")
	description, _ := reader.ReadString('\n')
	description = strings.TrimSpace(description)
	if description == "" {
		printError("description is required")
		return
	}

	version := "1.0.0"
	skill := map[string]any{
		"name":        name,
		"description": description,
		"version":     version,
		"commands":    []map[string]string{},
		"prompts":     map[string]string{},
	}

	data, err := json.MarshalIndent(skill, "", "  ")
	if err != nil {
		printError("failed to build skill file: %v", err)
		return
	}

	skillPath := filepath.Join(resolveSkillsDir(), name)
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		printError("failed to create skill dir: %v", err)
		return
	}
	filePath := filepath.Join(skillPath, "skill.json")
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		printError("failed to write skill file: %v", err)
		return
	}
	printSuccess("Skill created: %s", filePath)
}

func resolveSkillsDir() string {
	if envDir := strings.TrimSpace(os.Getenv("ANYCLAW_SKILLS_DIR")); envDir != "" {
		return envDir
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

func skillDisplayName(name, fullName string) string {
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

func printRemoteSkillResults(items []remoteSkillSearchItem) {
	fmt.Printf("Found %d skills\n\n", len(items))
	for i, item := range items {
		fmt.Printf("%d. %s\n", i+1, skillDisplayName(item.Name, item.FullName))
		fmt.Printf("   %s\n", skillDescription(item.Description))
		if strings.TrimSpace(item.Category) != "" {
			fmt.Printf("   category: %s\n", item.Category)
		}
		if strings.TrimSpace(item.Details) != "" {
			fmt.Printf("   %s\n", item.Details)
		}
		fmt.Printf("   install: %s\n\n", item.InstallCommand)
	}
}
