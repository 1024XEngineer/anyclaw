package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCandidatesReturnsClarifyForEmptyRequest(t *testing.T) {
	svc := NewServiceForRegistry(nil, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{
		Title: " ",
		Input: "\n",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}
	if candidates[0].Kind != CandidateClarify {
		t.Fatalf("expected clarify candidate, got %q", candidates[0].Kind)
	}
	if candidates[0].Confidence != 1 {
		t.Fatalf("expected full confidence, got %v", candidates[0].Confidence)
	}
}

func TestCandidatesMatchesKeywordRulesAndSortsByConfidence(t *testing.T) {
	svc := NewServiceForRegistry([]RouteRule{
		{
			ID:         "specialist-docs",
			Kind:       CandidateSpecialist,
			Keywords:   []string{"docs"},
			Confidence: 0.7,
			Reason:     "documentation route",
		},
		{
			ID:         "workflow-release",
			Kind:       CandidateWorkflow,
			Workflow:   "release",
			Keywords:   []string{"release", "deploy"},
			Confidence: 0.75,
			Reason:     "release workflow",
		},
		{
			ID:         "toolchain-ci",
			Kind:       CandidateToolchain,
			Path:       "tools/ci",
			Keywords:   []string{"ci"},
			Confidence: 0.95,
		},
	}, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{
		Title: "Release docs",
		Input: "please deploy the docs",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected two matching candidates, got %d: %#v", len(candidates), candidates)
	}
	if candidates[0].ID != "workflow-release" {
		t.Fatalf("expected release workflow first, got %q", candidates[0].ID)
	}
	if candidates[1].ID != "specialist-docs" {
		t.Fatalf("expected docs specialist second, got %q", candidates[1].ID)
	}
	if candidates[0].Workflow != "release" {
		t.Fatalf("expected workflow metadata to be copied, got %q", candidates[0].Workflow)
	}
}

func TestCandidatesUsesKindRankForConfidenceTies(t *testing.T) {
	svc := NewServiceForRegistry([]RouteRule{
		{
			ID:         "toolchain",
			Kind:       CandidateToolchain,
			Keywords:   []string{"route"},
			Confidence: 0.8,
		},
		{
			ID:         "workflow",
			Kind:       CandidateWorkflow,
			Keywords:   []string{"route"},
			Confidence: 0.8,
		},
	}, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{Input: "route this"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if candidates[0].Kind != CandidateWorkflow {
		t.Fatalf("expected workflow to win confidence tie, got %q", candidates[0].Kind)
	}
}

func TestCandidatesMatchesKeywordsWithTokenBoundaries(t *testing.T) {
	svc := NewServiceForRegistry([]RouteRule{
		{
			ID:         "toolchain-ci",
			Kind:       CandidateToolchain,
			Keywords:   []string{"ci"},
			Confidence: 0.9,
		},
	}, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{Input: "ask a specialist"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 1 || candidates[0].Kind != CandidateDirect {
		t.Fatalf("expected specialist not to match ci keyword, got %#v", candidates)
	}

	candidates, err = svc.Candidates(context.Background(), CandidateRequest{Input: "run ci, please"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "toolchain-ci" {
		t.Fatalf("expected standalone ci token to match, got %#v", candidates)
	}
}

func TestCandidatesMatchesMultiTokenKeywordPhrases(t *testing.T) {
	svc := NewServiceForRegistry([]RouteRule{
		{
			ID:         "triage-workflow",
			Kind:       CandidateWorkflow,
			Keywords:   []string{"open issue"},
			Confidence: 0.8,
		},
	}, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{Input: "open issue quickly"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if candidates[0].ID != "triage-workflow" {
		t.Fatalf("expected phrase keyword to match, got %#v", candidates)
	}

	candidates, err = svc.Candidates(context.Background(), CandidateRequest{Input: "open a new issue"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 1 || candidates[0].Kind != CandidateDirect {
		t.Fatalf("expected non-contiguous phrase not to match, got %#v", candidates)
	}
}

func TestCandidatesAddsApprovalForHighRiskInput(t *testing.T) {
	svc := NewServiceForRegistry([]RouteRule{
		{
			ID:         "production-workflow",
			Kind:       CandidateWorkflow,
			Keywords:   []string{"production"},
			Confidence: 0.9,
		},
	}, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{
		Input: "delete production credentials",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected approval and workflow candidates, got %d", len(candidates))
	}
	if candidates[0].Kind != CandidateApproval {
		t.Fatalf("expected approval candidate first, got %q", candidates[0].Kind)
	}
	if !candidates[0].RequiresApproval {
		t.Fatal("expected approval requirement to be set")
	}
	if candidates[0].RiskLevel != "high" {
		t.Fatalf("expected high risk level, got %q", candidates[0].RiskLevel)
	}
}

func TestCandidatesDetectsRiskWithTokenBoundaries(t *testing.T) {
	svc := NewServiceForRegistry(nil, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{
		Input: "message the secretary",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 1 || candidates[0].Kind != CandidateDirect {
		t.Fatalf("expected secretary not to trigger secret risk, got %#v", candidates)
	}

	candidates, err = svc.Candidates(context.Background(), CandidateRequest{
		Input: "remove all credentials",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 2 || candidates[0].Kind != CandidateApproval {
		t.Fatalf("expected bounded risk phrase to trigger approval plus fallback, got %#v", candidates)
	}
}

func TestCandidatesAddsDirectFallbackWhenHighRiskHasNoRunnableMatch(t *testing.T) {
	svc := NewServiceForRegistry(nil, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{
		Input: "delete production secrets",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected approval gate and direct fallback, got %d: %#v", len(candidates), candidates)
	}
	if candidates[0].Kind != CandidateApproval {
		t.Fatalf("expected approval candidate first, got %q", candidates[0].Kind)
	}
	if candidates[1].Kind != CandidateDirect {
		t.Fatalf("expected direct fallback after approval, got %q", candidates[1].Kind)
	}
}

func TestCandidatesFallsBackToDirectWhenNothingMatches(t *testing.T) {
	svc := NewServiceForRegistry([]RouteRule{
		{
			ID:       "workflow-release",
			Kind:     CandidateWorkflow,
			Keywords: []string{"release"},
		},
	}, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{Input: "say hello"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one fallback candidate, got %d", len(candidates))
	}
	if candidates[0].Kind != CandidateDirect {
		t.Fatalf("expected direct fallback, got %q", candidates[0].Kind)
	}
}

func TestCandidatesReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := NewServiceForRegistry(nil, nil)
	_, err := svc.Candidates(ctx, CandidateRequest{Input: "route"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestCandidatesRejectsNilServiceAndRouter(t *testing.T) {
	var svc *Service
	if _, err := svc.Candidates(context.Background(), CandidateRequest{Input: "route"}); err == nil {
		t.Fatal("expected nil service error")
	}

	var router *Router
	if _, err := router.Candidates(context.Background(), CandidateRequest{Input: "route"}); err == nil {
		t.Fatal("expected nil router error")
	}
}

func TestBuildGuidanceIncludesApprovalAndReason(t *testing.T) {
	guidance := BuildGuidance([]Candidate{
		{
			Kind:             CandidateApproval,
			ID:               "approval-high",
			Confidence:       1,
			RequiresApproval: true,
			RiskLevel:        "high",
			Reason:           "needs human review",
		},
	})

	for _, want := range []string{
		"Workflow route candidates:",
		"approval-high (approval, 1.00)",
		"approval required: high",
		"needs human review",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("expected guidance to contain %q, got:\n%s", want, guidance)
		}
	}
}

func TestAppendSuggestedSummary(t *testing.T) {
	candidates := []Candidate{{Kind: CandidateDirect, ID: "direct", Confidence: 0.55}}

	if got := AppendSuggestedSummary("", candidates); !strings.HasPrefix(got, "Workflow route candidates:") {
		t.Fatalf("expected guidance only for empty summary, got %q", got)
	}

	got := AppendSuggestedSummary("Existing summary", candidates)
	if !strings.Contains(got, "Existing summary\n\nWorkflow route candidates:") {
		t.Fatalf("expected summary and guidance, got %q", got)
	}
}

func TestDecideLLMUsesDefaultAndExplainsTopCandidate(t *testing.T) {
	decision := DecideLLM(nil, " ")
	if decision.Provider != "local" {
		t.Fatalf("expected local provider, got %q", decision.Provider)
	}
	if decision.Model != "default" {
		t.Fatalf("expected default model, got %q", decision.Model)
	}
	if decision.Reason != "default local routing model" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}

	decision = DecideLLM([]Candidate{{Kind: CandidateWorkflow, Confidence: 0.9}}, "router-small")
	if decision.Model != "router-small" {
		t.Fatalf("expected explicit model, got %q", decision.Model)
	}
	if decision.Reason != "top route candidate is workflow-oriented" {
		t.Fatalf("unexpected workflow reason: %q", decision.Reason)
	}

	decision = DecideLLM([]Candidate{{Kind: CandidateApproval, RequiresApproval: true}}, "router-small")
	if decision.Reason != "top route candidate requires approval" {
		t.Fatalf("unexpected approval reason: %q", decision.Reason)
	}
}

func TestNewRouterClonesRuleKeywords(t *testing.T) {
	rules := []RouteRule{{
		ID:       "workflow-release",
		Kind:     CandidateWorkflow,
		Keywords: []string{"release"},
	}}

	router := NewRouter(rules, nil)
	rules[0].Keywords[0] = "mutated"

	candidates, err := router.Candidates(context.Background(), CandidateRequest{Input: "release"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "workflow-release" {
		t.Fatalf("expected original release rule to match, got %#v", candidates)
	}
}

func TestRuleDefaultsAndConfidenceClamp(t *testing.T) {
	svc := NewServiceForRegistry([]RouteRule{
		{
			ID:         "default-kind",
			Keywords:   []string{"default"},
			Confidence: 1.5,
		},
		{
			ID:         "negative-confidence",
			Kind:       CandidateSpecialist,
			Keywords:   []string{"negative", "missing"},
			Confidence: -1,
		},
	}, nil)

	candidates, err := svc.Candidates(context.Background(), CandidateRequest{Input: "default negative"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if candidates[0].Kind != CandidateWorkflow {
		t.Fatalf("expected empty kind to default to workflow, got %q", candidates[0].Kind)
	}
	if candidates[0].Confidence != 1 {
		t.Fatalf("expected clamped confidence, got %v", candidates[0].Confidence)
	}
	if candidates[1].Confidence != 0.6 {
		t.Fatalf("expected zero base confidence to use default score, got %v", candidates[1].Confidence)
	}
	if candidates[1].Reason != "matched workflow route keywords" {
		t.Fatalf("expected default reason, got %q", candidates[1].Reason)
	}
}
