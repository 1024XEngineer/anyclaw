package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	desktopexec "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/desktop"
)

func TestExecuteProtocolOutputRunsDesktopSteps(t *testing.T) {
	registry := tools.NewRegistry()
	var called []string
	var approvals []tools.ToolApprovalCall
	registry.RegisterTool("desktop_open", "open desktop app", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "desktop_open:"+strings.TrimSpace(input["target"].(string)))
		return "opened", nil
	})
	registry.RegisterTool("desktop_type", "type desktop text", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "desktop_type:"+strings.TrimSpace(input["text"].(string)))
		return "typed", nil
	})

	payload, err := json.Marshal(desktopexec.DesktopPlan{
		Protocol: desktopexec.DesktopProtocolVersion,
		Summary:  "plan complete",
		Steps: []desktopexec.DesktopPlanStep{
			{Tool: "desktop_open", Label: "Launch", Input: map[string]any{"target": "demo.exe"}},
			{Tool: "desktop_type", Label: "Type", Input: map[string]any{"text": "hello"}},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	ctx := tools.WithToolApprovalHook(context.Background(), func(ctx context.Context, call tools.ToolApprovalCall) error {
		approvals = append(approvals, call)
		return nil
	})

	result, handled, err := ExecuteProtocolOutput(ctx, registry, ProtocolExecutionMeta{
		ToolName: "app_demo_run",
		Plugin:   "demo",
		App:      "Demo App",
		Action:   "run",
		Input:    map[string]any{"task": "hello"},
	}, payload)
	if err != nil {
		t.Fatalf("executeProtocolOutput: %v", err)
	}
	if !handled {
		t.Fatal("expected protocol output to be handled")
	}
	if len(approvals) != 1 {
		t.Fatalf("expected 1 protocol approval request, got %d", len(approvals))
	}
	if approvals[0].Name != "desktop_plan" {
		t.Fatalf("expected desktop_plan approval, got %q", approvals[0].Name)
	}
	if len(called) != 2 {
		t.Fatalf("expected 2 desktop calls, got %d", len(called))
	}
	if !strings.Contains(result, "plan complete") || !strings.Contains(result, "Launch: opened") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteProtocolOutputRetriesAndVerifiesSteps(t *testing.T) {
	registry := tools.NewRegistry()
	openCalls := 0
	verifyCalls := 0
	registry.RegisterTool("desktop_open", "open desktop app", nil, func(ctx context.Context, input map[string]any) (string, error) {
		openCalls++
		if openCalls == 1 {
			return "", fmt.Errorf("transient launch failure")
		}
		return "opened", nil
	})
	registry.RegisterTool("desktop_verify_text", "verify text", nil, func(ctx context.Context, input map[string]any) (string, error) {
		verifyCalls++
		if verifyCalls == 1 {
			return "", fmt.Errorf("text not visible yet")
		}
		return "verified", nil
	})

	payload, err := json.Marshal(desktopexec.DesktopPlan{
		Protocol: desktopexec.DesktopProtocolVersion,
		Summary:  "retry plan complete",
		Steps: []desktopexec.DesktopPlanStep{
			{
				Tool:  "desktop_open",
				Label: "Launch",
				Input: map[string]any{"target": "demo.exe"},
				Retry: 1,
				Verify: &desktopexec.DesktopPlanCheck{
					Tool:  "desktop_verify_text",
					Input: map[string]any{"expected": "ready"},
					Retry: 1,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, handled, err := ExecuteProtocolOutput(context.Background(), registry, ProtocolExecutionMeta{}, payload)
	if err != nil {
		t.Fatalf("executeProtocolOutput: %v", err)
	}
	if !handled {
		t.Fatal("expected protocol output to be handled")
	}
	if openCalls != 2 {
		t.Fatalf("expected 2 open attempts, got %d", openCalls)
	}
	if verifyCalls != 2 {
		t.Fatalf("expected 2 verify attempts, got %d", verifyCalls)
	}
	if !strings.Contains(result, "retry plan complete") || !strings.Contains(result, "Launch: opened") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteProtocolOutputCanContinueAfterFailure(t *testing.T) {
	registry := tools.NewRegistry()
	var called []string
	registry.RegisterTool("desktop_click", "click", nil, func(ctx context.Context, input map[string]any) (string, error) {
		return "", fmt.Errorf("button missing")
	})
	registry.RegisterTool("desktop_focus_window", "focus window", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "focus")
		return "focused", nil
	})
	registry.RegisterTool("desktop_type", "type text", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "type")
		return "typed", nil
	})

	payload, err := json.Marshal(desktopexec.DesktopPlan{
		Protocol: desktopexec.DesktopProtocolVersion,
		Summary:  "continued",
		Steps: []desktopexec.DesktopPlanStep{
			{
				Tool:            "desktop_click",
				Label:           "Try button",
				Input:           map[string]any{"x": 1, "y": 1},
				ContinueOnError: true,
				OnFailure: []desktopexec.DesktopPlanStep{
					{Tool: "desktop_focus_window", Label: "Refocus", Input: map[string]any{"title": "Demo"}},
				},
			},
			{
				Tool:  "desktop_type",
				Label: "Fallback typing",
				Input: map[string]any{"text": "hello"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, handled, err := ExecuteProtocolOutput(context.Background(), registry, ProtocolExecutionMeta{}, payload)
	if err != nil {
		t.Fatalf("executeProtocolOutput: %v", err)
	}
	if !handled {
		t.Fatal("expected protocol output to be handled")
	}
	if strings.Join(called, ",") != "focus,type" {
		t.Fatalf("unexpected recovery calls: %v", called)
	}
	if !strings.Contains(result, "Try button failed") || !strings.Contains(result, "Fallback typing: typed") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteProtocolOutputResumesFromSavedState(t *testing.T) {
	registry := tools.NewRegistry()
	var called []string
	registry.RegisterTool("desktop_open", "open desktop app", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "open")
		return "opened", nil
	})
	registry.RegisterTool("desktop_type", "type desktop text", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "type")
		return "typed", nil
	})

	payload, err := json.Marshal(desktopexec.DesktopPlan{
		Protocol: desktopexec.DesktopProtocolVersion,
		Summary:  "resume complete",
		Steps: []desktopexec.DesktopPlanStep{
			{Tool: "desktop_open", Label: "Launch", Input: map[string]any{"target": "demo.exe"}},
			{Tool: "desktop_type", Label: "Type", Input: map[string]any{"text": "hello"}},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var states []desktopexec.DesktopPlanExecutionState
	ctx := desktopexec.WithDesktopPlanResumeState(context.Background(), &desktopexec.DesktopPlanExecutionState{
		ToolName:          "app_demo_run",
		Plugin:            "demo",
		App:               "Demo App",
		Action:            "run",
		Status:            "failed",
		Summary:           "resume complete",
		TotalSteps:        2,
		NextStep:          2,
		LastCompletedStep: 1,
		LastOutput:        "Launch: opened",
		Steps: []desktopexec.DesktopPlanStepExecutionState{
			{Index: 1, Tool: "desktop_open", Label: "Launch", Status: "completed", Attempts: 1, Output: "Launch: opened"},
			{Index: 2, Tool: "desktop_type", Label: "Type", Status: "failed", Attempts: 1, Error: "typed failed"},
		},
	})
	ctx = desktopexec.WithDesktopPlanStateHook(ctx, func(ctx context.Context, state desktopexec.DesktopPlanExecutionState) {
		cloned := desktopexec.CloneDesktopPlanExecutionState(&state)
		if cloned != nil {
			states = append(states, *cloned)
		}
	})

	result, handled, err := ExecuteProtocolOutput(ctx, registry, ProtocolExecutionMeta{
		ToolName: "app_demo_run",
		Plugin:   "demo",
		App:      "Demo App",
		Action:   "run",
	}, payload)
	if err != nil {
		t.Fatalf("executeProtocolOutput: %v", err)
	}
	if !handled {
		t.Fatal("expected protocol output to be handled")
	}
	if strings.Join(called, ",") != "type" {
		t.Fatalf("expected only the remaining step to run, got %v", called)
	}
	if !strings.Contains(result, "Launch: opened") || !strings.Contains(result, "Type: typed") {
		t.Fatalf("unexpected resumed result: %q", result)
	}
	if len(states) == 0 {
		t.Fatal("expected plan states to be reported")
	}
	finalState := states[len(states)-1]
	if !finalState.Resumed {
		t.Fatalf("expected resumed state, got %#v", finalState)
	}
	if finalState.Status != "completed" {
		t.Fatalf("expected completed state, got %q", finalState.Status)
	}
	if finalState.LastCompletedStep != 2 || finalState.NextStep != 3 {
		t.Fatalf("unexpected checkpoint progression: %#v", finalState)
	}
}

func TestExecuteProtocolOutputSupportsStructuredTargetSteps(t *testing.T) {
	registry := tools.NewRegistry()
	var called []string
	registry.RegisterTool("desktop_activate_target", "activate target", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "activate:"+strings.TrimSpace(fmt.Sprint(input["title"]))+":"+strings.TrimSpace(fmt.Sprint(input["name"])))
		return "clicked", nil
	})
	registry.RegisterTool("desktop_set_target_value", "set target value", nil, func(ctx context.Context, input map[string]any) (string, error) {
		called = append(called, "set:"+strings.TrimSpace(fmt.Sprint(input["title"]))+":"+strings.TrimSpace(fmt.Sprint(input["value"])))
		return "typed", nil
	})

	value := "hello"
	payload, err := json.Marshal(desktopexec.DesktopPlan{
		Protocol: desktopexec.DesktopProtocolVersion,
		Summary:  "structured target complete",
		Steps: []desktopexec.DesktopPlanStep{
			{
				Label:  "Open composer",
				Target: map[string]any{"title": "QQ", "name": "消息输入框"},
				Action: "focus",
			},
			{
				Label:  "Type message",
				Target: map[string]any{"title": "QQ", "control_type": "edit"},
				Value:  &value,
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, handled, err := ExecuteProtocolOutput(context.Background(), registry, ProtocolExecutionMeta{}, payload)
	if err != nil {
		t.Fatalf("executeProtocolOutput: %v", err)
	}
	if !handled {
		t.Fatal("expected protocol output to be handled")
	}
	if strings.Join(called, ",") != "activate:QQ:消息输入框,set:QQ:hello" {
		t.Fatalf("unexpected target calls: %v", called)
	}
	if !strings.Contains(result, "structured target complete") || !strings.Contains(result, "Open composer: clicked") || !strings.Contains(result, "Type message: typed") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteProtocolOutputSupportsStructuredTargetVerification(t *testing.T) {
	registry := tools.NewRegistry()
	resolveCalls := 0
	registry.RegisterTool("desktop_activate_target", "activate target", nil, func(ctx context.Context, input map[string]any) (string, error) {
		return "clicked", nil
	})
	registry.RegisterTool("desktop_resolve_target", "resolve target", nil, func(ctx context.Context, input map[string]any) (string, error) {
		resolveCalls++
		if required, _ := input["require_found"].(bool); !required {
			t.Fatalf("expected target verification to require a found target")
		}
		if resolveCalls == 1 {
			return "", fmt.Errorf("target not found")
		}
		return `{"found":true,"strategy":"text"}`, nil
	})

	payload, err := json.Marshal(desktopexec.DesktopPlan{
		Protocol: desktopexec.DesktopProtocolVersion,
		Summary:  "verify target complete",
		Steps: []desktopexec.DesktopPlanStep{
			{
				Label:  "Click send",
				Target: map[string]any{"title": "QQ", "text": "发送"},
				Verify: &desktopexec.DesktopPlanCheck{
					Target:       map[string]any{"title": "QQ", "text": "已发送"},
					Retry:        1,
					RetryDelayMS: 1,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, handled, err := ExecuteProtocolOutput(context.Background(), registry, ProtocolExecutionMeta{}, payload)
	if err != nil {
		t.Fatalf("executeProtocolOutput: %v", err)
	}
	if !handled {
		t.Fatal("expected protocol output to be handled")
	}
	if resolveCalls != 2 {
		t.Fatalf("expected 2 resolve attempts, got %d", resolveCalls)
	}
	if !strings.Contains(result, "verify target complete") || !strings.Contains(result, "Click send: clicked") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteProtocolOutputSupportsStructuredTargetWaitStep(t *testing.T) {
	registry := tools.NewRegistry()
	resolveCalls := 0
	registry.RegisterTool("desktop_resolve_target", "resolve target", nil, func(ctx context.Context, input map[string]any) (string, error) {
		resolveCalls++
		if required, _ := input["require_found"].(bool); !required {
			t.Fatalf("expected wait target step to require a found target")
		}
		if resolveCalls == 1 {
			return "", fmt.Errorf("target not ready")
		}
		return `{"found":true,"strategy":"ui"}`, nil
	})

	payload, err := json.Marshal(desktopexec.DesktopPlan{
		Protocol: desktopexec.DesktopProtocolVersion,
		Summary:  "wait target complete",
		Steps: []desktopexec.DesktopPlanStep{
			{
				Label:        "Wait for send button",
				Target:       map[string]any{"title": "QQ", "text": "发送"},
				Action:       "wait",
				Retry:        1,
				RetryDelayMS: 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	result, handled, err := ExecuteProtocolOutput(context.Background(), registry, ProtocolExecutionMeta{}, payload)
	if err != nil {
		t.Fatalf("executeProtocolOutput: %v", err)
	}
	if !handled {
		t.Fatal("expected protocol output to be handled")
	}
	if resolveCalls != 2 {
		t.Fatalf("expected 2 resolve attempts, got %d", resolveCalls)
	}
	if !strings.Contains(result, "wait target complete") || !strings.Contains(result, "Wait for send button:") {
		t.Fatalf("unexpected result: %q", result)
	}
}
