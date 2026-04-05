package clihub

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type Command struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

type Skill struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Commands    []Command `json:"commands"`
	Groups      []string  `json:"groups,omitempty"`
	SourcePath  string    `json:"source_path"`
}

var skillFilePattern = regexp.MustCompile(`(?i)skills?[/\\]SKILL\.md$`)

func LoadSkillFor(status EntryStatus) (*Skill, error) {
	if status.SkillPath == "" {
		return nil, fmt.Errorf("no skill path available for %s", status.Name)
	}
	return LoadSkill(status.SkillPath, status.SourcePath)
}

func LoadSkill(skillPath string, sourcePath string) (*Skill, error) {
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, err
	}
	skill := &Skill{
		SourcePath: sourcePath,
	}
	if err := parseSkillMarkdown(string(data), skill); err != nil {
		return nil, err
	}
	return skill, nil
}

func parseSkillMarkdown(content string, skill *Skill) error {
	lines := strings.Split(content, "\n")

	inFrontmatter := false
	frontmatter := []string{}
	afterFrontmatter := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && trimmed == "---" {
			inFrontmatter = false
			afterFrontmatter = true
			continue
		}
		if inFrontmatter {
			frontmatter = append(frontmatter, line)
			continue
		}
		if afterFrontmatter {
			if err := parseYAMLFrontmatter(strings.Join(frontmatter, "\n"), skill); err != nil {
				return err
			}
			afterFrontmatter = false
		}

		if strings.HasPrefix(trimmed, "## ") {
			group := strings.TrimPrefix(trimmed, "## ")
			group = strings.TrimSuffix(group, " Group")
			group = strings.TrimSpace(group)
			skill.Groups = append(skill.Groups, group)
		}
	}

	if err := parseCommandTables(content, skill); err != nil {
		return err
	}

	return nil
}

func parseYAMLFrontmatter(frontmatter string, skill *Skill) error {
	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			skill.Name = strings.ReplaceAll(skill.Name, ">-", "")
			skill.Name = strings.ReplaceAll(skill.Name, ">", "")
			skill.Name = strings.TrimSpace(skill.Name)
		}
		if strings.HasPrefix(line, "description:") {
			skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			skill.Description = strings.ReplaceAll(skill.Description, ">-", "")
			skill.Description = strings.ReplaceAll(skill.Description, ">", "")
			skill.Description = strings.TrimSpace(skill.Description)
		}
	}
	return nil
}

func parseCommandTables(content string, skill *Skill) error {
	lines := strings.Split(content, "\n")
	currentGroup := ""

	tableStartRe := regexp.MustCompile(`^\|\s*Command\s*\|`)
	tableRowRe := regexp.MustCompile(`^\|\s*(\S+)\s*\|(.*)\|`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## ") {
			group := strings.TrimPrefix(trimmed, "## ")
			group = strings.TrimSuffix(group, " Group")
			group = strings.TrimSpace(group)

			skipGroups := []string{"command groups", "command", "groups", "usage", "basic commands", "examples", "for ai agents", "more information", "version", "state management", "output formats", "installation", "prerequisites"}

			lowerGroup := strings.ToLower(group)
			skip := false
			for _, sg := range skipGroups {
				if strings.Contains(lowerGroup, sg) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			if group != "" {
				currentGroup = group
			}
			continue
		}

		if tableStartRe.MatchString(trimmed) {
			for j := i + 1; j < len(lines); j++ {
				row := strings.TrimSpace(lines[j])
				if row == "" || !strings.HasPrefix(row, "|") {
					break
				}
				match := tableRowRe.FindStringSubmatch(row)
				if match != nil {
					cmdName := strings.TrimSpace(match[1])
					cmdName = strings.ReplaceAll(cmdName, "`", "")
					desc := strings.TrimSpace(match[2])
					desc = strings.ReplaceAll(desc, "`", "")
					if isMarkdownSeparatorRow(cmdName) || isMarkdownSeparatorRow(desc) {
						continue
					}
					if cmdName != "Command" && cmdName != "" {
						skill.Commands = append(skill.Commands, Command{
							Name:        cmdName,
							Group:       currentGroup,
							Description: desc,
						})
					}
				}
			}
		}
	}

	return nil
}

func isMarkdownSeparatorRow(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		switch r {
		case '-', ':', '|', ' ':
		default:
			return false
		}
	}
	return true
}

func LoadSkillsForCatalog(cat *Catalog) map[string]*Skill {
	result := make(map[string]*Skill)
	if cat == nil {
		return result
	}
	for _, entry := range cat.Entries {
		status := statusForEntry(cat.Root, entry)
		if status.SkillPath == "" {
			continue
		}
		skill, err := LoadSkill(status.SkillPath, status.SourcePath)
		if err != nil {
			continue
		}
		result[entry.Name] = skill
	}
	return result
}

func FindSkillPath(root string, name string) string {
	entry := Entry{Name: name}
	return skillPath(root, entry)
}

func SkillFileExists(root string, name string) bool {
	return FindSkillPath(root, name) != ""
}

func HumanSkillSummary(skill *Skill) string {
	if skill == nil {
		return ""
	}
	lines := []string{
		fmt.Sprintf("%s: %s", skill.Name, skill.Description),
	}
	if len(skill.Commands) > 0 {
		cmdNames := make([]string, 0, len(skill.Commands))
		for _, cmd := range skill.Commands {
			cmdNames = append(cmdNames, cmd.Name)
		}
		lines = append(lines, fmt.Sprintf("commands: %s", strings.Join(cmdNames, ", ")))
	}
	return strings.Join(lines, "\n")
}

func FindSkill(cat *Catalog, name string) (*Skill, error) {
	status, ok := Find(cat, name)
	if !ok {
		return nil, fmt.Errorf("entry not found: %s", name)
	}
	if status.SkillPath == "" {
		return nil, fmt.Errorf("no skill for %s", name)
	}
	return LoadSkillFor(status)
}

func AllSkills(cat *Catalog) []*Skill {
	var result []*Skill
	if cat == nil {
		return result
	}
	for _, entry := range cat.Entries {
		status := statusForEntry(cat.Root, entry)
		if status.SkillPath == "" {
			continue
		}
		skill, err := LoadSkillFor(status)
		if err != nil {
			continue
		}
		result = append(result, skill)
	}
	return result
}

func CommandCount(skill *Skill) int {
	if skill == nil {
		return 0
	}
	return len(skill.Commands)
}
