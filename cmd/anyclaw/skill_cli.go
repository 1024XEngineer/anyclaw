package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

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
	if len(os.Args) < 3 {
		printSkillhubUsage()
		return
	}

	args := os.Args[2:]
	switch args[0] {
	case "search":
		query := ""
		if len(args) > 1 {
			query = strings.Join(args[1:], " ")
		}
		searchSkillhubFromCLI(query)
	case "install":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: anyclaw skillhub install <name>")
			os.Exit(1)
		}
		installSkillhubFromCLI(args[1])
	case "list":
		listSkillhubSkills()
	case "check":
		checkSkillhubCLI()
	default:
		fmt.Fprintf(os.Stderr, "unknown skillhub command: %s\n", args[0])
		printSkillhubUsage()
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

func printSkillhubUsage() {
	fmt.Print(`AnyClaw skillhub commands:

Usage:
  anyclaw skillhub search <query>
  anyclaw skillhub install <name>
  anyclaw skillhub list
  anyclaw skillhub check
`)
}

func searchSkillhubFromCLI(query string) {
	fmt.Printf("Searching Skillhub: %s\n", query)
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

	fmt.Printf("Found %d skills\n\n", len(results))
	for i, r := range results {
		fullName := r.FullName
		if fullName == "" {
			fullName = r.Name
		}
		desc := r.Description
		if desc == "" {
			desc = "No description"
		}
		fmt.Printf("%d. %s\n", i+1, fullName)
		fmt.Printf("   %s\n", desc)
		if r.Category != "" {
			fmt.Printf("   category: %s\n", r.Category)
		}
		fmt.Printf("   install: anyclaw skillhub install %s\n\n", r.Name)
	}
}

func installSkillhubFromCLI(skillName string) {
	fmt.Printf("Installing skillhub skill: %s\n", skillName)
	ctx := context.Background()
	skillsDir := "skills"
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		printError("failed to create skills dir: %v", err)
		return
	}
	if err := skills.InstallSkillhubSkill(ctx, skillName, skillsDir); err != nil {
		printError("install failed: %v", err)
		return
	}
	printSuccess("Installed skillhub skill: %s", skillName)
}

func listSkillhubSkills() {
	entries, err := os.ReadDir("skills")
	if err != nil {
		printInfo("No installed skills.")
		return
	}
	var list []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join("skills", entry.Name(), "skill.json")); err == nil {
			list = append(list, entry.Name())
		}
	}
	if len(list) == 0 {
		printInfo("No installed skills.")
		return
	}
	fmt.Printf("%s\n\n", ui.Bold.Sprint("Installed skills"))
	for _, name := range list {
		fmt.Printf("  - %s\n", name)
	}
}

func checkSkillhubCLI() {
	printSuccess("Skillhub CLI is available")
	printInfo("Use `anyclaw skillhub search <query>` to search")
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

	fmt.Printf("Found %d skills\n\n", len(results))
	for i, r := range results {
		fullName := r.FullName
		if fullName == "" {
			fullName = r.Name
		}
		desc := r.Description
		if desc == "" {
			desc = "No description"
		}
		installs := formatInstalls(r.Installs)
		fmt.Printf("%d. %s\n", i+1, fullName)
		fmt.Printf("   %s\n", desc)
		fmt.Printf("   installs: %s  stars: %d  %s\n", installs, r.Stars, getQualityBadge(r.Installs, r.Stars))
		fmt.Printf("   install: anyclaw skill install %s\n\n", r.Name)
	}
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
	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}

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
	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}
	manager := skills.NewSkillsManager(skillsDir)
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
	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}
	manager := skills.NewSkillsManager(skillsDir)
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
		name := entry.FullName
		if name == "" {
			name = entry.Name
		}
		fmt.Printf("- %s v%s\n", name, entry.Version)
		fmt.Printf("  %s\n", entry.Description)
	}
}

func createNewSkill() {
	reader := bufio.NewReader(os.Stdin)
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

	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}
	skillPath := filepath.Join(skillsDir, name)
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
