package orchestrator

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultDecomposeScoresSkillsBeforeFirstAgent(t *testing.T) {
	decomposer := NewTaskDecomposer(nil)
	agents := []AgentCapability{
		{
			Name:        "generalist",
			Description: "General purpose worker",
		},
		{
			Name:        "security-reviewer",
			Description: "Reviews authorization and governance changes",
			Domain:      "security",
			Expertise:   []string{"permissions", "governance"},
			Skills:      []string{"go-audit"},
		},
	}

	plan := decomposer.defaultDecompose("task_1", "请使用 go-audit 检查权限模型", agents)
	if len(plan.SubTasks) != 1 {
		t.Fatalf("expected one sub-task, got %#v", plan.SubTasks)
	}
	task := plan.SubTasks[0]
	if task.AssignedAgent != "security-reviewer" {
		t.Fatalf("expected skill-aware assignment to security-reviewer, got %#v", task)
	}
	if task.AssignmentScore <= 0 {
		t.Fatalf("expected positive assignment score, got %#v", task)
	}
	if !strings.Contains(task.AssignmentReason, "skill:go-audit") {
		t.Fatalf("expected skill match in assignment reason, got %q", task.AssignmentReason)
	}
}

func TestCandidateWindowFiltersMissingToolCategories(t *testing.T) {
	agents := []AgentCapability{
		{
			Name:           "file-reader",
			Description:    "Reads local files",
			ToolCategories: []string{"file"},
		},
		{
			Name:           "browser-operator",
			Description:    "Operates browser sessions",
			ToolCategories: []string{"browser"},
			Tools:          []string{"browser_navigate"},
		},
	}

	window := selectCandidateWindow(rankAgentCandidates("打开浏览器页面", agents), 4)
	if len(window) != 1 {
		t.Fatalf("expected one candidate after hard tool filtering, got %#v", window)
	}
	if window[0].Capability.Name != "browser-operator" {
		t.Fatalf("expected browser operator, got %#v", window)
	}
}

func TestDecomposePromptIncludesToolsAndFallbacksInvalidPlannerAgent(t *testing.T) {
	planner := &capturePlanner{response: `{
		"summary": "open the page",
		"sub_tasks": [{
			"title": "Open page",
			"description": "Use browser_navigate to open the requested page",
			"assigned_agent": "ghost-agent",
			"required_capabilities": ["browser"],
			"confidence": 0.9,
			"reason": "planner guess"
		}]
	}`}
	decomposer := NewTaskDecomposer(planner)
	agents := []AgentCapability{
		{
			Name:        "generalist",
			Description: "General purpose worker",
		},
		{
			Name:            "browser-operator",
			Description:     "Operates browser sessions",
			Skills:          []string{"page-check"},
			Tools:           []string{"browser_navigate"},
			ToolCategories:  []string{"browser"},
			PermissionLevel: "limited",
		},
	}

	plan, err := decomposer.Decompose(context.Background(), "task_2", "使用 browser_navigate 打开页面", agents)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(plan.SubTasks) != 1 {
		t.Fatalf("expected one sub-task, got %#v", plan.SubTasks)
	}
	task := plan.SubTasks[0]
	if task.AssignedAgent != "browser-operator" {
		t.Fatalf("expected fallback to browser-operator, got %#v", task)
	}
	if !strings.Contains(task.AssignmentReason, "unavailable agent") {
		t.Fatalf("expected invalid planner agent to be audited, got %q", task.AssignmentReason)
	}
	prompt := planner.userPrompt()
	for _, want := range []string{"skills=page-check", "tools=browser_navigate", "tool_categories=browser", "permission=limited"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to include %q, got:\n%s", want, prompt)
		}
	}
}

func TestLowConfidencePlannerSelectionFallsBackToScoredAgent(t *testing.T) {
	planner := &capturePlanner{response: `{
		"summary": "review permissions",
		"sub_tasks": [{
			"title": "Review authorization",
			"description": "Security permissions review",
			"assigned_agent": "generalist",
			"confidence": 0.2
		}]
	}`}
	decomposer := NewTaskDecomposer(planner)
	agents := []AgentCapability{
		{
			Name:        "generalist",
			Description: "General purpose worker",
		},
		{
			Name:        "security-reviewer",
			Description: "Reviews security-sensitive permission changes",
			Domain:      "security",
			Expertise:   []string{"permissions"},
		},
	}

	plan, err := decomposer.Decompose(context.Background(), "task_3", "security permissions review", agents)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if got := plan.SubTasks[0].AssignedAgent; got != "security-reviewer" {
		t.Fatalf("expected low-confidence planner selection to fall back to security-reviewer, got %q", got)
	}
	if !strings.Contains(plan.SubTasks[0].AssignmentReason, "confidence") {
		t.Fatalf("expected confidence fallback reason, got %q", plan.SubTasks[0].AssignmentReason)
	}
}

func TestRequiredCapabilitiesOverridePlannerAgentWhenDescriptionIsAmbiguous(t *testing.T) {
	planner := &capturePlanner{response: `{
		"summary": "perform browser work",
		"sub_tasks": [{
			"title": "Handle browser step",
			"description": "Complete the requested step",
			"assigned_agent": "generalist",
			"required_capabilities": ["tool_category:browser"],
			"confidence": 0.9
		}]
	}`}
	decomposer := NewTaskDecomposer(planner)
	agents := []AgentCapability{
		{
			Name:        "generalist",
			Description: "General purpose worker",
		},
		{
			Name:           "browser-operator",
			Description:    "Operates browser sessions",
			ToolCategories: []string{"browser"},
		},
	}

	plan, err := decomposer.Decompose(context.Background(), "task_4", "do the step", agents)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if got := plan.SubTasks[0].AssignedAgent; got != "browser-operator" {
		t.Fatalf("expected required browser capability to select browser-operator, got %q", got)
	}
	if !strings.Contains(plan.SubTasks[0].AssignmentReason, "missed inferred tool or skill requirements") {
		t.Fatalf("expected required capability fallback reason, got %q", plan.SubTasks[0].AssignmentReason)
	}
	if got := plan.SubTasks[0].RequiredCapabilities; len(got) != 1 || got[0] != "tool_category:browser" {
		t.Fatalf("expected planner required capability to be preserved, got %#v", got)
	}
}

type capturePlanner struct {
	response string
	messages []interface{}
}

func (p *capturePlanner) Chat(ctx context.Context, messages []interface{}, tools []interface{}) (*PlannerResponse, error) {
	p.messages = messages
	return &PlannerResponse{Content: p.response}, nil
}

func (p *capturePlanner) Name() string {
	return "capture"
}

func (p *capturePlanner) userPrompt() string {
	for i := len(p.messages) - 1; i >= 0; i-- {
		msg, ok := p.messages[i].(map[string]string)
		if !ok || msg["role"] != "user" {
			continue
		}
		return msg["content"]
	}
	return ""
}
