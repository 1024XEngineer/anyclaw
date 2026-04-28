package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type CandidateKind string

const (
	CandidateDirect     CandidateKind = "direct"
	CandidateWorkflow   CandidateKind = "workflow"
	CandidateSpecialist CandidateKind = "specialist"
	CandidateToolchain  CandidateKind = "toolchain"
	CandidateClarify    CandidateKind = "clarify"
	CandidateApproval   CandidateKind = "approval"
)

type CandidateRequest struct {
	ID         string
	Input      string
	Title      string
	UserID     string
	Org        string
	Project    string
	Workspace  string
	SessionID  string
	ConfigPath string
}

type Candidate struct {
	Kind             CandidateKind
	Path             string
	ID               string
	Plugin           string
	Workflow         string
	App              string
	Confidence       float64
	RequiresApproval bool
	RiskLevel        string
	Reason           string
}

type RouteRule struct {
	ID               string
	Kind             CandidateKind
	Path             string
	Plugin           string
	Workflow         string
	App              string
	Keywords         []string
	Confidence       float64
	RequiresApproval bool
	RiskLevel        string
	Reason           string
}

type PlannerClient interface{}

type LLMRouteDecision struct {
	Provider string
	Model    string
	Reason   string
}

type Router struct {
	rules   []RouteRule
	planner PlannerClient
}

func NewRouter(rules []RouteRule, planner PlannerClient) *Router {
	return &Router{
		rules:   cloneRules(rules),
		planner: planner,
	}
}

type Service struct {
	router *Router
}

func NewService(router *Router) *Service {
	if router == nil {
		router = NewRouter(nil, nil)
	}
	return &Service{router: router}
}

func NewServiceForRegistry(rules []RouteRule, planner PlannerClient) *Service {
	return NewService(NewRouter(rules, planner))
}

func (s *Service) Candidates(ctx context.Context, req CandidateRequest) ([]Candidate, error) {
	if s == nil || s.router == nil {
		return nil, fmt.Errorf("workflow route service is nil")
	}
	return s.router.Candidates(ctx, req)
}

func (r *Router) Candidates(ctx context.Context, req CandidateRequest) ([]Candidate, error) {
	if r == nil {
		return nil, fmt.Errorf("workflow route router is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	input := strings.TrimSpace(req.Title + " " + req.Input)
	if input == "" {
		return []Candidate{{
			Kind:       CandidateClarify,
			ID:         "clarify-empty-request",
			Confidence: 1,
			Reason:     "request is empty; ask for the task goal before routing",
		}}, nil
	}

	candidates := make([]Candidate, 0, len(r.rules)+2)
	for _, rule := range r.rules {
		score, matched := matchRule(rule, input)
		if !matched {
			continue
		}
		candidates = append(candidates, candidateFromRule(rule, score))
	}

	if risk := detectRisk(input); risk != "" {
		candidates = append(candidates, Candidate{
			Kind:             CandidateApproval,
			ID:               "approval-" + risk,
			Confidence:       1,
			RequiresApproval: true,
			RiskLevel:        risk,
			Reason:           "request appears to involve a high-impact action",
		})
	}

	if !hasRunnableCandidate(candidates) {
		candidates = append(candidates, Candidate{
			Kind:       CandidateDirect,
			ID:         "direct-default",
			Confidence: 0.55,
			Reason:     "no workflow rule matched; route to the default direct handler",
		})
	}

	sortCandidates(candidates)
	return candidates, nil
}

func BuildGuidance(candidates []Candidate) string {
	if len(candidates) == 0 {
		return "No workflow route candidates were found."
	}

	var lines []string
	lines = append(lines, "Workflow route candidates:")
	for i, candidate := range candidates {
		label := strings.TrimSpace(candidate.ID)
		if label == "" {
			label = string(candidate.Kind)
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s, %.2f)", i+1, label, candidate.Kind, candidate.Confidence))
		if candidate.RequiresApproval {
			lines = append(lines, fmt.Sprintf("   approval required: %s", candidate.RiskLevel))
		}
		if reason := strings.TrimSpace(candidate.Reason); reason != "" {
			lines = append(lines, "   "+reason)
		}
	}
	return strings.Join(lines, "\n")
}

func AppendSuggestedSummary(summary string, candidates []Candidate) string {
	guidance := BuildGuidance(candidates)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return guidance
	}
	return summary + "\n\n" + guidance
}

func DecideLLM(candidates []Candidate, model string) LLMRouteDecision {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "default"
	}

	decision := LLMRouteDecision{
		Provider: "local",
		Model:    model,
		Reason:   "default local routing model",
	}
	if len(candidates) == 0 {
		return decision
	}
	if candidates[0].RequiresApproval {
		decision.Reason = "top route candidate requires approval"
		return decision
	}
	if candidates[0].Kind == CandidateWorkflow || candidates[0].Kind == CandidateToolchain {
		decision.Reason = "top route candidate is workflow-oriented"
	}
	return decision
}

func matchRule(rule RouteRule, input string) (float64, bool) {
	keywords := normalizedKeywords(rule.Keywords)
	if len(keywords) == 0 {
		return 0, false
	}

	inputTokens := tokenizeText(input)
	matches := 0
	for _, keyword := range keywords {
		if containsNormalizedPhrase(inputTokens, keyword) {
			matches++
		}
	}
	if matches == 0 {
		return 0, false
	}

	base := clampConfidence(rule.Confidence)
	if base == 0 {
		base = 0.6
	}
	score := base + float64(matches-1)*0.08
	if matches == len(keywords) {
		score += 0.08
	}
	return clampConfidence(score), true
}

func candidateFromRule(rule RouteRule, score float64) Candidate {
	kind := rule.Kind
	if kind == "" {
		kind = CandidateWorkflow
	}
	reason := strings.TrimSpace(rule.Reason)
	if reason == "" {
		reason = "matched workflow route keywords"
	}
	return Candidate{
		Kind:             kind,
		Path:             strings.TrimSpace(rule.Path),
		ID:               strings.TrimSpace(rule.ID),
		Plugin:           strings.TrimSpace(rule.Plugin),
		Workflow:         strings.TrimSpace(rule.Workflow),
		App:              strings.TrimSpace(rule.App),
		Confidence:       score,
		RequiresApproval: rule.RequiresApproval,
		RiskLevel:        strings.TrimSpace(rule.RiskLevel),
		Reason:           reason,
	}
}

func hasRunnableCandidate(candidates []Candidate) bool {
	for _, candidate := range candidates {
		if candidate.Kind != CandidateApproval && candidate.Kind != CandidateClarify {
			return true
		}
	}
	return false
}

func detectRisk(input string) string {
	inputTokens := tokenizeText(input)
	highRiskWords := []string{
		"delete",
		"drop",
		"wipe",
		"remove all",
		"production",
		"payment",
		"credential",
		"secret",
	}
	for _, word := range highRiskWords {
		if containsNormalizedPhrase(inputTokens, word) {
			return "high"
		}
	}
	return ""
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Confidence != candidates[j].Confidence {
			return candidates[i].Confidence > candidates[j].Confidence
		}
		return candidateKindRank(candidates[i].Kind) < candidateKindRank(candidates[j].Kind)
	})
}

func candidateKindRank(kind CandidateKind) int {
	switch kind {
	case CandidateApproval:
		return 0
	case CandidateWorkflow:
		return 1
	case CandidateToolchain:
		return 2
	case CandidateSpecialist:
		return 3
	case CandidateDirect:
		return 4
	case CandidateClarify:
		return 5
	default:
		return 6
	}
}

func normalizedKeywords(keywords []string) []string {
	normalized := make([]string, 0, len(keywords))
	seen := make(map[string]bool, len(keywords))
	for _, keyword := range keywords {
		keyword = normalizeText(keyword)
		if keyword == "" || seen[keyword] {
			continue
		}
		seen[keyword] = true
		normalized = append(normalized, keyword)
	}
	return normalized
}

func containsNormalizedPhrase(inputTokens []string, phrase string) bool {
	phraseTokens := tokenizeText(phrase)
	if len(phraseTokens) == 0 || len(inputTokens) < len(phraseTokens) {
		return false
	}
	for i := 0; i <= len(inputTokens)-len(phraseTokens); i++ {
		matched := true
		for j, token := range phraseTokens {
			if inputTokens[i+j] != token {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func normalizeText(input string) string {
	return strings.Join(tokenizeText(input), " ")
}

func tokenizeText(input string) []string {
	return strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func clampConfidence(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func cloneRules(rules []RouteRule) []RouteRule {
	if len(rules) == 0 {
		return nil
	}
	cloned := make([]RouteRule, len(rules))
	for i, rule := range rules {
		rule.Keywords = append([]string(nil), rule.Keywords...)
		cloned[i] = rule
	}
	return cloned
}
