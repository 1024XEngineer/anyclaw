package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/clihub"
)

const (
	CapabilityTaskSimple         = "simple"
	CapabilityTaskLocalExecution = "local_execution"
	CapabilityTaskComplex        = "complex"
	CapabilityTaskSpecialized    = "specialized"
	CapabilityTaskUnknown        = "unknown"

	CapabilityRouteMainAgent     = "main_agent"
	CapabilityRouteUseSkill      = "use_skill"
	CapabilityRouteDelegateAgent = "delegate_agent"
	CapabilityRouteUseCLI        = "use_cli"
	CapabilityRouteSearchMarket  = "search_market"
	CapabilityRouteNone          = "none"
)

type CapabilityPlanner struct{}

type CapabilityPlannerInput struct {
	UserInput       string
	Tools           []tools.ToolInfo
	Skills          []skills.SkillCatalogEntry
	Agents          []CapabilityAgentSummary
	CLICapabilities []clihub.Capability
	LocalArtifacts  []CapabilityArtifactSummary
}

type CapabilityAgentSummary struct {
	Name        string
	Description string
	Domain      string
	Expertise   []string
	Skills      []string
	Tools       []string
}

type CapabilityArtifactSummary struct {
	ArtifactID   string
	Kind         string
	Name         string
	Description  string
	Capabilities []string
	Installed    bool
	Bound        bool
}

type CapabilityMatch struct {
	Kind   string  `json:"kind"`
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
}

type CapabilityPlan struct {
	TaskClass                string            `json:"task_class"`
	Route                    string            `json:"route"`
	Need                     string            `json:"need,omitempty"`
	KindHint                 string            `json:"kind_hint,omitempty"`
	LocalMatches             []CapabilityMatch `json:"local_matches,omitempty"`
	ShouldExposeMarketSearch bool              `json:"should_expose_market_search,omitempty"`
	Reason                   string            `json:"reason,omitempty"`
}

func (p CapabilityPlanner) Plan(input CapabilityPlannerInput) CapabilityPlan {
	normalized := normalizeCapabilityPlannerText(input.UserInput)
	if strings.TrimSpace(normalized) == "" {
		return CapabilityPlan{TaskClass: CapabilityTaskUnknown, Route: CapabilityRouteNone, Reason: "empty request"}
	}

	if isSimpleCapabilityRequest(normalized) {
		return CapabilityPlan{TaskClass: CapabilityTaskSimple, Route: CapabilityRouteMainAgent, Need: strings.TrimSpace(input.UserInput), Reason: "simple conversational request"}
	}

	need := detectCapabilityPlannerNeed(input.UserInput)
	if need == "" {
		need = strings.TrimSpace(input.UserInput)
	}

	if match := bestSkillCapabilityMatch(normalized, input.Skills); match.Score >= 0.35 {
		return CapabilityPlan{
			TaskClass:    CapabilityTaskSpecialized,
			Route:        CapabilityRouteUseSkill,
			Need:         need,
			KindHint:     "skill",
			LocalMatches: []CapabilityMatch{match},
			Reason:       "matching local skill is available",
		}
	}

	if match := bestAgentCapabilityMatch(normalized, input.Agents); match.Score >= 0.35 {
		return CapabilityPlan{
			TaskClass:    CapabilityTaskSpecialized,
			Route:        CapabilityRouteDelegateAgent,
			Need:         need,
			KindHint:     "agent",
			LocalMatches: []CapabilityMatch{match},
			Reason:       "matching local agent is available",
		}
	}

	if match := bestCLICapabilityMatch(normalized, input.CLICapabilities); match.Score >= 0.35 {
		return CapabilityPlan{
			TaskClass:    CapabilityTaskLocalExecution,
			Route:        CapabilityRouteUseCLI,
			Need:         need,
			KindHint:     "cli",
			LocalMatches: []CapabilityMatch{match},
			Reason:       "matching CLIHub capability is available",
		}
	}

	if match := bestArtifactCapabilityMatch(normalized, input.LocalArtifacts); match.Score >= 0.35 {
		return CapabilityPlan{
			TaskClass:    CapabilityTaskSpecialized,
			Route:        routeForLocalArtifact(match.Kind),
			Need:         need,
			KindHint:     match.Kind,
			LocalMatches: []CapabilityMatch{match},
			Reason:       "matching installed marketplace artifact is available locally",
		}
	}

	intent := classifyCodexToolIntent(normalized, input.UserInput, false)
	if specialized, kindHint := inferSpecializedCapabilityNeed(normalized); specialized {
		return CapabilityPlan{
			TaskClass:                CapabilityTaskSpecialized,
			Route:                    CapabilityRouteSearchMarket,
			Need:                     need,
			KindHint:                 kindHint,
			ShouldExposeMarketSearch: true,
			Reason:                   "specialized capability is not available locally",
		}
	}

	if capabilityPlannerHasLocalExecutionIntent(normalized, intent) {
		return CapabilityPlan{
			TaskClass: CapabilityTaskLocalExecution,
			Route:     CapabilityRouteMainAgent,
			Need:      need,
			Reason:    "request fits built-in local execution tools",
		}
	}

	if capabilityPlannerLooksComplex(normalized) {
		return CapabilityPlan{
			TaskClass: CapabilityTaskComplex,
			Route:     CapabilityRouteMainAgent,
			Need:      need,
			Reason:    "complex task can start with main-agent planning and local tools",
		}
	}

	return CapabilityPlan{TaskClass: CapabilityTaskUnknown, Route: CapabilityRouteMainAgent, Need: need, Reason: "no specialized local or marketplace route required"}
}

func detectCapabilityPlannerNeed(input string) string {
	normalized := normalizeCapabilityPlannerText(input)
	switch {
	case capabilityPlannerContainsAny(normalized, "release notes", "changelog", "version notes"):
		return "release notes"
	case capabilityPlannerContainsAny(normalized, "code review", "review code", "pull request", "pr review"):
		return "code review"
	case capabilityPlannerContainsAny(normalized, "repo health", "repository health", "diagnose repo"):
		return "repo health"
	case capabilityPlannerContainsAny(normalized, "video render", "render video", "timeline", "shotcut", "premiere"):
		return "video editing workflow"
	case capabilityPlannerContainsAny(normalized, "spreadsheet", "excel", "presentation", "slides", "powerpoint"):
		return "office document workflow"
	default:
		return strings.TrimSpace(input)
	}
}

func bestSkillCapabilityMatch(query string, items []skills.SkillCatalogEntry) CapabilityMatch {
	var best CapabilityMatch
	for _, item := range items {
		haystack := strings.Join([]string{item.Name, item.FullName, item.Description, item.Category, item.Registry, item.Source, strings.Join(item.Permissions, " ")}, " ")
		score := capabilityPlannerScore(query, haystack)
		if score > best.Score {
			best = CapabilityMatch{Kind: "skill", Name: firstCapabilityPlannerNonEmpty(item.Name, item.FullName), Score: score, Reason: item.Description}
		}
	}
	return best
}

func bestAgentCapabilityMatch(query string, items []CapabilityAgentSummary) CapabilityMatch {
	var best CapabilityMatch
	for _, item := range items {
		haystack := strings.Join([]string{item.Name, item.Description, item.Domain, strings.Join(item.Expertise, " "), strings.Join(item.Skills, " "), strings.Join(item.Tools, " ")}, " ")
		score := capabilityPlannerScore(query, haystack)
		if score > best.Score {
			best = CapabilityMatch{Kind: "agent", Name: item.Name, Score: score, Reason: firstCapabilityPlannerNonEmpty(item.Description, item.Domain)}
		}
	}
	return best
}

func bestCLICapabilityMatch(query string, items []clihub.Capability) CapabilityMatch {
	var best CapabilityMatch
	for _, item := range items {
		haystack := strings.Join([]string{item.Harness, item.Group, item.Command, item.Action, item.Category, strings.Join(item.Keywords, " ")}, " ")
		score := capabilityPlannerScore(query, haystack)
		if score > best.Score {
			best = CapabilityMatch{Kind: "cli", Name: strings.TrimSpace(item.Harness + " " + item.Command), Score: score, Reason: item.Action}
		}
	}
	return best
}

func bestArtifactCapabilityMatch(query string, items []CapabilityArtifactSummary) CapabilityMatch {
	var best CapabilityMatch
	for _, item := range items {
		if !item.Installed {
			continue
		}
		haystack := strings.Join([]string{item.ArtifactID, item.Kind, item.Name, item.Description, strings.Join(item.Capabilities, " ")}, " ")
		score := capabilityPlannerScore(query, haystack)
		if item.Bound {
			score += 0.05
		}
		if score > best.Score {
			best = CapabilityMatch{Kind: item.Kind, Name: firstCapabilityPlannerNonEmpty(item.Name, item.ArtifactID), Score: score, Reason: item.Description}
		}
	}
	return best
}

func capabilityPlannerScore(query string, haystack string) float64 {
	queryTerms := capabilityPlannerTerms(query)
	if len(queryTerms) == 0 {
		return 0
	}
	haystack = normalizeCapabilityPlannerText(haystack)
	if haystack == "" {
		return 0
	}
	matches := 0
	for _, term := range queryTerms {
		if strings.Contains(haystack, term) {
			matches++
		}
	}
	score := float64(matches) / float64(len(queryTerms))
	if strings.Contains(haystack, query) {
		score += 0.25
	}
	if score > 1 {
		return 1
	}
	return score
}

func capabilityPlannerTerms(input string) []string {
	input = normalizeCapabilityPlannerText(input)
	terms := strings.Fields(input)
	out := make([]string, 0, len(terms))
	seen := map[string]struct{}{}
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if len(term) < 3 || capabilityPlannerStopWords[term] {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	sort.Strings(out)
	return out
}

var capabilityPlannerStopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "this": true, "that": true,
	"please": true, "help": true, "need": true, "want": true, "make": true, "create": true,
	"帮我": true, "请": true,
}

func isSimpleCapabilityRequest(normalized string) bool {
	if capabilityPlannerContainsAny(normalized, "who are you", "what are you", "thank you") || capabilityPlannerHasAnyWholeTerm(normalized, "hello", "hi", "thanks") {
		return true
	}
	if specialized, _ := inferSpecializedCapabilityNeed(normalized); specialized {
		return false
	}
	if len(strings.Fields(normalized)) <= 4 && !capabilityPlannerContainsAny(normalized, "create", "write", "edit", "run", "open", "install", "review", "render", "generate") {
		return true
	}
	return false
}

func capabilityPlannerHasLocalExecutionIntent(query string, intent codexToolIntent) bool {
	if intent.File || intent.Write || intent.Command || intent.Web || intent.Fetch || intent.Image || intent.Memory || intent.Plan || intent.Status || intent.Desktop || intent.Browser || intent.Automation {
		return true
	}
	return capabilityPlannerContainsAny(query, "file", "folder", "command", "terminal", "website", "url", "local app", "desktop")
}

func inferSpecializedCapabilityNeed(query string) (bool, string) {
	switch {
	case capabilityPlannerContainsAny(query, "code review", "pull request", "pr review", "security audit"):
		return true, "agent"
	case capabilityPlannerContainsAny(query, "release notes", "changelog", "version notes", "technical writing"):
		return true, "skill"
	case capabilityPlannerContainsAny(query, "repo health", "repository health", "dependency audit", "diagnose repository"):
		return true, "cli"
	case capabilityPlannerContainsAny(query, "shotcut", "premiere", "video timeline", "render video", "audio mastering"):
		return true, "cli"
	case capabilityPlannerContainsAny(query, "spreadsheet", "excel macro", "powerpoint", "slides", "presentation deck"):
		return true, "skill"
	default:
		return false, ""
	}
}

func capabilityPlannerLooksComplex(query string) bool {
	return strings.Count(query, " ") >= 12 || capabilityPlannerContainsAny(query, "architecture", "multi step", "end to end", "workflow", "pipeline", "orchestrate")
}

func routeForLocalArtifact(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "agent":
		return CapabilityRouteDelegateAgent
	case "skill":
		return CapabilityRouteUseSkill
	case "cli":
		return CapabilityRouteUseCLI
	default:
		return CapabilityRouteMainAgent
	}
}

func normalizeCapabilityPlannerText(input string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ", `\`, " ", ".", " ", ":", " ", ",", " ", ";", " ", "?", " ", "!", " ", "\n", " ", "\t", " ")
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(replacer.Replace(input)))), " ")
}

func capabilityPlannerContainsAny(value string, needles ...string) bool {
	value = normalizeCapabilityPlannerText(value)
	for _, needle := range needles {
		needle = normalizeCapabilityPlannerText(needle)
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func capabilityPlannerHasAnyWholeTerm(value string, terms ...string) bool {
	fields := strings.Fields(normalizeCapabilityPlannerText(value))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		seen[field] = struct{}{}
	}
	for _, term := range terms {
		if _, ok := seen[normalizeCapabilityPlannerText(term)]; ok {
			return true
		}
	}
	return false
}

func firstCapabilityPlannerNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (p CapabilityPlan) Summary() string {
	parts := []string{fmt.Sprintf("%s via %s", p.TaskClass, p.Route)}
	if p.Need != "" {
		parts = append(parts, "need="+p.Need)
	}
	if p.KindHint != "" {
		parts = append(parts, "kind="+p.KindHint)
	}
	if p.Reason != "" {
		parts = append(parts, p.Reason)
	}
	return strings.Join(parts, "; ")
}
