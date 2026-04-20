package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type PreferenceLearner struct {
	workingDir string
}

type LearnedPreference struct {
	Name        string
	Description string
	Style       string
	Boundary    string
}

var (
	namePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?:我叫|名字是|你可以叫我|叫我)\s*([^，。,\s]+)`),
		regexp.MustCompile(`(?:我的名字|名字)\s*叫\s*([^，。,\s]+)`),
	}
	stylePatterns = []*regexp.Regexp{
		regexp.MustCompile(`喜欢(?:用|说|看|听)(.*?)[，。,\s]|$`),
		regexp.MustCompile(`(?:风格|语气|方式)\s*(?:是|要)?(.*?)[，。,\s]|$`),
	}
	boundaryPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?:不要|别|尽量不要|尽量避免)(.*?)[，。,\s]|$`),
	}
)

func NewPreferenceLearner(workingDir string) *PreferenceLearner {
	return &PreferenceLearner{workingDir: workingDir}
}

func (p *PreferenceLearner) Learn(userInput, assistantResponse string) (string, bool) {
	if p.workingDir == "" {
		return "", false
	}

	pref := p.extractPreferences(userInput)
	if pref.Name == "" && pref.Style == "" && pref.Boundary == "" {
		return "", false
	}

	changed := false
	var messages []string

	if pref.Name != "" {
		if p.updateAgentName(pref.Name) {
			messages = append(messages, fmt.Sprintf("好的，以后叫我「%s」", pref.Name))
			changed = true
		}
	}

	if pref.Style != "" {
		if p.updateUserStyle(pref.Style) {
			messages = append(messages, fmt.Sprintf("知道了，我会%s", pref.Style))
			changed = true
		}
	}

	if pref.Boundary != "" {
		if p.updateBoundary(pref.Boundary) {
			messages = append(messages, fmt.Sprintf("好的，我会%s", pref.Boundary))
			changed = true
		}
	}

	if len(messages) > 0 {
		return strings.Join(messages, "，") + "。", changed
	}
	return "", false
}

func (p *PreferenceLearner) extractPreferences(userInput string) LearnedPreference {
	pref := LearnedPreference{}

	for _, pattern := range namePatterns {
		if match := pattern.FindStringSubmatch(userInput); len(match) > 1 {
			pref.Name = strings.TrimSpace(match[1])
			break
		}
	}

	for _, pattern := range stylePatterns {
		if match := pattern.FindStringSubmatch(userInput); len(match) > 1 {
			pref.Style = strings.TrimSpace(match[1])
			break
		}
	}

	for _, pattern := range boundaryPatterns {
		if match := pattern.FindStringSubmatch(userInput); len(match) > 1 {
			pref.Boundary = strings.TrimSpace(match[1])
			break
		}
	}

	return pref
}

func (p *PreferenceLearner) updateAgentName(name string) bool {
	agentsFile := filepath.Join(p.workingDir, "AGENTS.md")
	if _, err := os.Stat(agentsFile); err != nil {
		return false
	}

	content, err := os.ReadFile(agentsFile)
	if err != nil {
		return false
	}

	oldContent := string(content)
	newContent := strings.Replace(oldContent,
		"- Name: "+strings.Split(strings.Split(oldContent, "- Name: ")[1], "\n")[0],
		"- Name: "+name,
		1)

	if newContent == oldContent {
		return false
	}

	return os.WriteFile(agentsFile, []byte(newContent), 0644) == nil
}

func (p *PreferenceLearner) updateUserStyle(style string) bool {
	userFile := filepath.Join(p.workingDir, "USER.md")
	if _, err := os.Stat(userFile); err != nil {
		return false
	}

	content, err := os.ReadFile(userFile)
	if err != nil {
		return false
	}

	oldContent := string(content)
	styleLine := fmt.Sprintf("- Style: %s", style)

	if strings.Contains(oldContent, styleLine) {
		return false
	}

	newContent := oldContent + "\n" + styleLine

	return os.WriteFile(userFile, []byte(newContent), 0644) == nil
}

func (p *PreferenceLearner) updateBoundary(boundary string) bool {
	userFile := filepath.Join(p.workingDir, "USER.md")
	if _, err := os.Stat(userFile); err != nil {
		return false
	}

	content, err := os.ReadFile(userFile)
	if err != nil {
		return false
	}

	oldContent := string(content)
	boundaryLine := fmt.Sprintf("- Avoid: %s", boundary)

	if strings.Contains(oldContent, boundaryLine) {
		return false
	}

	newContent := oldContent + "\n" + boundaryLine

	return os.WriteFile(userFile, []byte(newContent), 0644) == nil
}
