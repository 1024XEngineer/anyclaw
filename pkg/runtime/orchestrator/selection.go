package orchestrator

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	defaultAgentCandidateLimit = 4
	minPlannerConfidence       = 0.45
)

type taskRequirements struct {
	ToolCategories []string
	Tools          []string
	Skills         []string
	Mutation       bool
}

type agentCandidateScore struct {
	Capability AgentCapability
	Score      int
	Confidence float64
	Matched    []string
	Missing    []string
}

type agentAssignment struct {
	Name         string
	Score        int
	Confidence   float64
	Reason       string
	RequiredCaps []string
}

func rankAgentCandidates(input string, agents []AgentCapability) []agentCandidateScore {
	req := inferTaskRequirements(input, agents)
	scores := make([]agentCandidateScore, 0, len(agents))
	for _, agent := range agents {
		scores = append(scores, scoreAgentCandidate(input, req, agent))
	}
	sort.SliceStable(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	return scores
}

func selectCandidateWindow(scores []agentCandidateScore, limit int) []agentCandidateScore {
	if len(scores) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = defaultAgentCandidateLimit
	}

	filtered := make([]agentCandidateScore, 0, len(scores))
	for _, score := range scores {
		if len(score.Missing) == 0 {
			filtered = append(filtered, score)
		}
	}
	if len(filtered) == 0 {
		filtered = scores
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func candidatesToCapabilities(scores []agentCandidateScore) []AgentCapability {
	agents := make([]AgentCapability, 0, len(scores))
	for _, score := range scores {
		agents = append(agents, score.Capability)
	}
	return agents
}

func bestAgentAssignment(input string, agents []AgentCapability) agentAssignment {
	scores := rankAgentCandidates(input, agents)
	if len(scores) == 0 {
		return agentAssignment{}
	}
	best := scores[0]
	return agentAssignment{
		Name:         best.Capability.Name,
		Score:        best.Score,
		Confidence:   best.Confidence,
		Reason:       best.assignmentReason(),
		RequiredCaps: inferredCapabilityLabels(input, agents),
	}
}

func chooseAgentAssignment(requested string, description string, plannerConfidence float64, requiredCaps []string, candidates []AgentCapability) agentAssignment {
	scoringInput := strings.TrimSpace(description)
	if len(requiredCaps) > 0 {
		scoringInput = strings.TrimSpace(scoringInput + " " + strings.Join(requiredCaps, " "))
	}
	scores := rankAgentCandidates(scoringInput, candidates)
	if len(scores) == 0 {
		return agentAssignment{}
	}
	best := scores[0]
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return assignmentFromScore(best, "planner did not select an available agent")
	}

	var requestedScore agentCandidateScore
	found := false
	for _, score := range scores {
		if score.Capability.Name == requested {
			requestedScore = score
			found = true
			break
		}
	}
	if !found {
		return assignmentFromScore(best, fmt.Sprintf("planner selected unavailable agent %q", requested))
	}
	if plannerConfidence > 0 && plannerConfidence < minPlannerConfidence && requestedScore.Capability.Name != best.Capability.Name {
		return assignmentFromScore(best, fmt.Sprintf("planner confidence %.2f below %.2f", plannerConfidence, minPlannerConfidence))
	}
	if len(requestedScore.Missing) > 0 && len(best.Missing) == 0 && requestedScore.Capability.Name != best.Capability.Name {
		return assignmentFromScore(best, "planner selection missed inferred tool or skill requirements")
	}
	return assignmentFromScore(requestedScore, "")
}

func assignmentFromScore(score agentCandidateScore, prefix string) agentAssignment {
	reason := score.assignmentReason()
	if strings.TrimSpace(prefix) != "" {
		reason = prefix + "; " + reason
	}
	return agentAssignment{
		Name:         score.Capability.Name,
		Score:        score.Score,
		Confidence:   score.Confidence,
		Reason:       reason,
		RequiredCaps: score.requiredCapabilityLabels(),
	}
}

func inferTaskRequirements(input string, agents []AgentCapability) taskRequirements {
	text := normalizeCapabilityText(input)
	req := taskRequirements{}
	addCategory := func(category string) {
		if !containsString(req.ToolCategories, category) {
			req.ToolCategories = append(req.ToolCategories, category)
		}
	}
	addTool := func(tool string) {
		if !containsString(req.Tools, tool) {
			req.Tools = append(req.Tools, tool)
		}
	}
	addSkill := func(skill string) {
		if !containsString(req.Skills, skill) {
			req.Skills = append(req.Skills, skill)
		}
	}

	if capabilityTextMatchesAny(text, "browser", "playwright", "tab", "page", "浏览器", "网页", "页面") {
		addCategory("browser")
	}
	if strings.Contains(input, "http://") || strings.Contains(input, "https://") ||
		capabilityTextMatchesAny(text, "web", "search", "internet", "online", "latest", "fetch url", "url", "网站", "搜索", "联网", "最新") {
		addCategory("web")
	}
	if capabilityTextMatchesAny(text, "shell", "terminal", "command", "run command", "execute", "build", "compile", "test", "go test", "npm", "pnpm", "yarn", "pytest", "命令", "终端", "运行", "执行", "测试", "构建", "编译") {
		addCategory("command")
	}
	if capabilityTextMatchesAny(text, "file", "folder", "directory", "repo", "code", "patch", "read file", "write file", "edit", "modify", "fix", "implement", "文件", "目录", "代码", "补丁", "修改", "修复", "实现", "读取", "写入") {
		addCategory("file")
	}
	if capabilityTextMatchesAny(text, "desktop", "window", "click", "ocr", "screenshot", "ui automation", "桌面", "窗口", "点击", "截图") {
		addCategory("desktop")
	}
	if capabilityTextMatchesAny(text, "memory", "remember", "recall", "记忆", "记住", "回忆") {
		addCategory("memory")
	}
	if capabilityTextMatchesAny(text, "write", "edit", "modify", "delete", "fix", "implement", "create", "update", "remove", "写", "改", "修改", "修复", "实现", "删除", "创建", "更新") {
		req.Mutation = true
	}

	for _, agent := range agents {
		for _, tool := range agent.Tools {
			if capabilityContainsNormalized(text, tool) {
				addTool(tool)
			}
		}
		for _, skill := range agent.Skills {
			if capabilityContainsNormalized(text, skill) {
				addSkill(skill)
			}
		}
	}

	return req
}

func scoreAgentCandidate(input string, req taskRequirements, agent AgentCapability) agentCandidateScore {
	text := normalizeCapabilityText(input)
	score := 0
	matched := make([]string, 0)
	missing := make([]string, 0)

	addMatch := func(label string, points int) {
		if label == "" {
			return
		}
		score += points
		if !containsString(matched, label) {
			matched = append(matched, label)
		}
	}
	addMissing := func(label string) {
		if label != "" && !containsString(missing, label) {
			missing = append(missing, label)
		}
	}

	if capabilityContainsNormalized(text, agent.Name) {
		addMatch("name:"+agent.Name, 2)
	}
	if capabilityContainsNormalized(text, agent.Domain) {
		addMatch("domain:"+agent.Domain, 5)
	}
	for _, expertise := range agent.Expertise {
		if capabilityContainsNormalized(text, expertise) {
			addMatch("expertise:"+expertise, 4)
		}
	}
	for _, skill := range agent.Skills {
		if capabilityContainsNormalized(text, skill) {
			addMatch("skill:"+skill, 6)
		}
	}

	for _, category := range req.ToolCategories {
		if containsFold(agent.ToolCategories, category) {
			addMatch("tool_category:"+category, 5)
		} else {
			score -= 6
			addMissing("tool_category:" + category)
		}
	}
	for _, tool := range req.Tools {
		if containsFold(agent.Tools, tool) {
			addMatch("tool:"+tool, 6)
		} else {
			score -= 8
			addMissing("tool:" + tool)
		}
	}
	for _, skill := range req.Skills {
		if containsFold(agent.Skills, skill) {
			addMatch("required_skill:"+skill, 7)
		} else {
			score -= 8
			addMissing("skill:" + skill)
		}
	}
	if req.Mutation && strings.EqualFold(strings.TrimSpace(agent.PermissionLevel), "read-only") {
		score -= 6
		addMissing("permission:mutation")
	}

	profileText := normalizeCapabilityText(strings.Join([]string{
		agent.Description,
		agent.Domain,
		strings.Join(agent.Expertise, " "),
		strings.Join(agent.Keywords, " "),
	}, " "))
	for _, term := range significantTerms(text) {
		if len([]rune(term)) < 3 {
			continue
		}
		if capabilityContainsNormalized(profileText, term) {
			addMatch("keyword:"+term, 1)
		}
	}

	return agentCandidateScore{
		Capability: agent,
		Score:      score,
		Confidence: confidenceForScore(score, missing),
		Matched:    matched,
		Missing:    missing,
	}
}

func confidenceForScore(score int, missing []string) float64 {
	if score <= 0 {
		if len(missing) > 0 {
			return 0.15
		}
		return 0.25
	}
	confidence := 0.35 + float64(score)/30.0
	if len(missing) > 0 {
		confidence -= 0.15
	}
	if confidence < 0.1 {
		return 0.1
	}
	if confidence > 0.95 {
		return 0.95
	}
	return confidence
}

func (s agentCandidateScore) assignmentReason() string {
	parts := make([]string, 0, 2)
	if len(s.Matched) > 0 {
		parts = append(parts, "matched "+strings.Join(limitStrings(s.Matched, 5), ", "))
	}
	if len(s.Missing) > 0 {
		parts = append(parts, "missing "+strings.Join(limitStrings(s.Missing, 4), ", "))
	}
	if len(parts) == 0 {
		return "selected by stable fallback"
	}
	return strings.Join(parts, "; ")
}

func (s agentCandidateScore) requiredCapabilityLabels() []string {
	labels := make([]string, 0, len(s.Matched)+len(s.Missing))
	for _, item := range append(append([]string{}, s.Matched...), s.Missing...) {
		if strings.HasPrefix(item, "tool_category:") || strings.HasPrefix(item, "tool:") || strings.HasPrefix(item, "required_skill:") || strings.HasPrefix(item, "skill:") || strings.HasPrefix(item, "permission:") {
			labels = append(labels, item)
		}
	}
	return limitStrings(labels, 8)
}

func inferredCapabilityLabels(input string, agents []AgentCapability) []string {
	req := inferTaskRequirements(input, agents)
	labels := make([]string, 0, len(req.ToolCategories)+len(req.Tools)+len(req.Skills)+1)
	for _, category := range req.ToolCategories {
		labels = append(labels, "tool_category:"+category)
	}
	for _, tool := range req.Tools {
		labels = append(labels, "tool:"+tool)
	}
	for _, skill := range req.Skills {
		labels = append(labels, "skill:"+skill)
	}
	if req.Mutation {
		labels = append(labels, "permission:mutation")
	}
	return labels
}

func formatAgentCandidateForPrompt(score agentCandidateScore) string {
	a := score.Capability
	details := make([]string, 0, 8)
	if strings.TrimSpace(a.Domain) != "" {
		details = append(details, "domain="+a.Domain)
	}
	if len(a.Expertise) > 0 {
		details = append(details, "expertise="+strings.Join(limitStrings(a.Expertise, 6), ", "))
	}
	if len(a.Skills) > 0 {
		details = append(details, "skills="+strings.Join(limitStrings(a.Skills, 8), ", "))
	}
	if len(a.ToolCategories) > 0 {
		details = append(details, "tool_categories="+strings.Join(limitStrings(a.ToolCategories, 8), ", "))
	}
	if len(a.Tools) > 0 {
		details = append(details, "tools="+strings.Join(limitStrings(a.Tools, 12), ", "))
	}
	if strings.TrimSpace(a.PermissionLevel) != "" {
		details = append(details, "permission="+a.PermissionLevel)
	}
	details = append(details, fmt.Sprintf("selection_score=%d", score.Score))
	if len(score.Matched) > 0 {
		details = append(details, "matched="+strings.Join(limitStrings(score.Matched, 5), ", "))
	}
	if len(score.Missing) > 0 {
		details = append(details, "missing="+strings.Join(limitStrings(score.Missing, 4), ", "))
	}

	description := strings.TrimSpace(a.Description)
	if description == "" {
		description = "No description"
	}
	return fmt.Sprintf("- %s: %s (%s)", a.Name, description, strings.Join(details, "; "))
}

func normalizeCapabilityText(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastSpace := true
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func capabilityTextMatchesAny(text string, terms ...string) bool {
	for _, term := range terms {
		if capabilityContainsNormalized(text, term) {
			return true
		}
	}
	return false
}

func capabilityContainsNormalized(normalizedText string, term string) bool {
	normalizedTerm := normalizeCapabilityText(term)
	if normalizedTerm == "" {
		return false
	}
	if isShortASCII(normalizedTerm) {
		for _, field := range strings.Fields(normalizedText) {
			if field == normalizedTerm {
				return true
			}
		}
		return false
	}
	if strings.Contains(normalizedText, normalizedTerm) {
		return true
	}
	return strings.Contains(strings.ReplaceAll(normalizedText, " ", ""), strings.ReplaceAll(normalizedTerm, " ", ""))
}

func isShortASCII(term string) bool {
	if len([]rune(term)) > 2 {
		return false
	}
	for _, r := range term {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func significantTerms(normalizedText string) []string {
	raw := strings.Fields(normalizedText)
	terms := make([]string, 0, len(raw))
	for _, term := range raw {
		if isRoutingStopWord(term) {
			continue
		}
		if !containsString(terms, term) {
			terms = append(terms, term)
		}
	}
	return terms
}

func isRoutingStopWord(term string) bool {
	switch term {
	case "the", "and", "for", "with", "this", "that", "please", "task", "agent", "use", "using", "to", "of", "in", "on", "a", "an":
		return true
	case "请", "帮", "帮我", "任务", "使用", "一个", "这个", "那个":
		return true
	default:
		return false
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsFold(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func limitStrings(items []string, limit int) []string {
	if limit <= 0 || len(items) <= limit {
		return append([]string(nil), items...)
	}
	limited := append([]string(nil), items[:limit]...)
	limited = append(limited, fmt.Sprintf("+%d more", len(items)-limit))
	return limited
}
