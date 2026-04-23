package mcp

import (
	"context"
	"strings"
	"testing"

	toolregistry "github.com/1024XEngineer/anyclaw/pkg/capability/tools"
)

func TestRegistryAndBridge(t *testing.T) {
	ctx := context.Background()
	registry := NewRegistry()

	client := newHelperClient(t, "")
	if err := registry.Register("helper", client); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := registry.Register("helper", client); err == nil {
		t.Fatal("expected duplicate register error")
	}

	if errs := registry.ConnectAll(ctx); len(errs) != 0 {
		t.Fatalf("ConnectAll: %#v", errs)
	}
	defer registry.DisconnectAll()

	if got := registry.List(); len(got) != 1 || got[0] != "helper" {
		t.Fatalf("unexpected registry list: %#v", got)
	}
	if got, ok := registry.Get("helper"); !ok || got == nil {
		t.Fatalf("expected to get registered client, got %#v %v", got, ok)
	}
	if got := registry.AllTools(); len(got["helper"]) != 1 {
		t.Fatalf("unexpected tools map: %#v", got)
	}
	if got := registry.AllResources(); len(got["helper"]) != 1 {
		t.Fatalf("unexpected resources map: %#v", got)
	}
	if got := registry.AllPrompts(); len(got["helper"]) != 1 {
		t.Fatalf("unexpected prompts map: %#v", got)
	}

	status := registry.Status()["helper"]
	if !status.Connected || status.Tools != 1 || status.Resources != 1 || status.Prompts != 1 {
		t.Fatalf("unexpected registry status: %#v", status)
	}

	result, err := registry.CallTool(ctx, "helper", "echo", map[string]any{"message": "bridge"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if text := firstContentText(result); text != "tool:bridge" {
		t.Fatalf("unexpected tool response text: %q", text)
	}
	if _, err := registry.ReadResource(ctx, "helper", "resource://status"); err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if _, err := registry.GetPrompt(ctx, "helper", "review", map[string]string{"focus": "safety"}); err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}

	toolReg := toolregistry.NewRegistry()
	if err := BridgeToToolRegistry(toolReg, registry); err != nil {
		t.Fatalf("BridgeToToolRegistry: %v", err)
	}

	out, err := toolReg.Call(ctx, "mcp__helper__echo", map[string]any{"message": "tool-registry"})
	if err != nil {
		t.Fatalf("tool registry call: %v", err)
	}
	if out != "tool:tool-registry" {
		t.Fatalf("unexpected bridged output: %q", out)
	}

	registry.Remove("helper")
	if _, ok := registry.Get("helper"); ok {
		t.Fatal("expected removed client to be absent")
	}
}

func TestRegistryErrorsAndFormatting(t *testing.T) {
	registry := NewRegistry()

	if _, err := registry.CallTool(context.Background(), "missing", "echo", nil); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing client error, got %v", err)
	}

	client := NewClient("helper", "unused", nil, nil)
	if err := registry.Register("helper", client); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := registry.CallTool(context.Background(), "helper", "echo", nil); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("expected not connected error, got %v", err)
	}
	if _, err := registry.ReadResource(context.Background(), "helper", "resource://status"); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("expected not connected resource error, got %v", err)
	}
	if _, err := registry.GetPrompt(context.Background(), "helper", "review", nil); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("expected not connected prompt error, got %v", err)
	}

	if got, err := formatMCPResult(nil); err != nil || got != "" {
		t.Fatalf("unexpected nil format result: %q %v", got, err)
	}
	if got, err := formatMCPResult(map[string]any{
		"content": []any{
			map[string]any{"text": "part1"},
			map[string]any{"text": "part2"},
		},
	}); err != nil || got != "part1\n\npart2" {
		t.Fatalf("unexpected content format result: %q %v", got, err)
	}
	if got, err := formatMCPResult(map[string]any{"value": "x"}); err != nil || !strings.Contains(got, "\"value\": \"x\"") {
		t.Fatalf("unexpected JSON format result: %q %v", got, err)
	}
	if got := joinStrings([]string{"a", "b", "c"}, "/"); got != "a/b/c" {
		t.Fatalf("unexpected joined string: %q", got)
	}
}
