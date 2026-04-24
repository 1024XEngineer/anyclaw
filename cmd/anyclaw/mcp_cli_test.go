package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/mcp"
)

func TestMCPCLIHelperProcess(t *testing.T) {
	if cliHelperMode() == "" {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req mcp.Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := cliHelperResponse(req)
		if resp != nil {
			_ = encoder.Encode(resp)
		}
	}

	os.Exit(0)
}

func TestRunAnyClawCLIRoutesMCPToolsAndUsesConfiguredServers(t *testing.T) {
	configPath := writeMCPCLIConfig(t)

	stdout, stderr, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"mcp", "tools", "--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI: %v\nstderr=%s", err, stderr)
	}

	var toolsList []listedMCPTool
	if err := json.Unmarshal([]byte(stdout), &toolsList); err != nil {
		t.Fatalf("unmarshal output: %v\nstdout=%s", err, stdout)
	}

	if !containsListedTool(toolsList, "AnyClaw", "chat") {
		t.Fatalf("expected builtin chat tool in output, got %#v", toolsList)
	}
	if !containsListedTool(toolsList, "helper", "echo") {
		t.Fatalf("expected configured helper tool in output, got %#v", toolsList)
	}
}

func TestRunAnyClawCLIUnknownCommandPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw commands:") {
		t.Fatalf("expected CLI usage output, got %q", stdout)
	}
}

func TestRunAnyClawCLIHelpPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI help: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw commands:") {
		t.Fatalf("expected CLI usage output, got %q", stdout)
	}
}

func TestRunAnyClawCLINoArgsPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI(nil)
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI no args: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw commands:") {
		t.Fatalf("expected CLI usage output, got %q", stdout)
	}
}

func TestRunAnyClawCLIRoutesPluginCommand(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"plugin"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI plugin: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw plugin commands:") {
		t.Fatalf("expected plugin usage output, got %q", stdout)
	}
}

func TestRunMCPCommandWithoutArgsPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runMCPCommand(nil)
	})
	if err != nil {
		t.Fatalf("runMCPCommand: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw MCP commands:") {
		t.Fatalf("expected MCP usage output, got %q", stdout)
	}
}

func TestRunMCPCommandUnknownSubcommandPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runMCPCommand([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown mcp command") {
		t.Fatalf("expected unknown subcommand error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw MCP commands:") {
		t.Fatalf("expected MCP usage output, got %q", stdout)
	}
}

func TestRunMCPServeStartsBuiltinServer(t *testing.T) {
	configPath := writeDefaultCLIConfig(t)

	stdout, stderr, err := withCLIStdin(t, "", func() (string, string, error) {
		return captureCLIOutput(t, func() error {
			return runMCPServe([]string{"--config", configPath})
		})
	})
	if err == nil || !strings.Contains(err.Error(), "stdin closed") {
		t.Fatalf("expected stdin closed error, got %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "mcp server: anyclaw MCP server started (stdio)") {
		t.Fatalf("expected server startup message, got stderr=%q", stderr)
	}
}

func TestRunMCPToolsTextOutputIncludesSources(t *testing.T) {
	configPath := writeMCPCLIConfig(t)

	stdout, stderr, err := captureCLIOutput(t, func() error {
		return runMCPTools([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runMCPTools: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "[AnyClaw] chat") {
		t.Fatalf("expected builtin tool in text output, got %q", stdout)
	}
	if !strings.Contains(stdout, "[helper] echo") {
		t.Fatalf("expected configured tool in text output, got %q", stdout)
	}
}

func TestAnyClawMainHelperProcess(t *testing.T) {
	mode := os.Getenv("ANYCLAW_MAIN_HELPER")
	if mode == "" {
		return
	}

	os.Args = []string{"anyclaw"}
	if mode == "success" {
		os.Args = append(os.Args, "help")
	} else {
		os.Args = append(os.Args, "unknown")
	}
	main()
}

func TestMainExecutesCLIAndPropagatesErrors(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=TestAnyClawMainHelperProcess")
		cmd.Env = append(os.Environ(), "ANYCLAW_MAIN_HELPER=success")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("expected success exit, got %v\noutput=%s", err, output)
		}
		if !strings.Contains(string(output), "AnyClaw commands:") {
			t.Fatalf("expected CLI usage output, got %q", output)
		}
	})

	t.Run("error", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=TestAnyClawMainHelperProcess")
		cmd.Env = append(os.Environ(), "ANYCLAW_MAIN_HELPER=error")
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected non-zero exit for error path, got output=%s", output)
		}
		if !strings.Contains(string(output), "unknown command: unknown") {
			t.Fatalf("expected unknown command output, got %q", output)
		}
	})
}

func captureCLIOutput(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	runErr := fn()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	stdoutBytes, _ := io.ReadAll(stdoutR)
	stderrBytes, _ := io.ReadAll(stderrR)
	_ = stdoutR.Close()
	_ = stderrR.Close()

	return string(stdoutBytes), string(stderrBytes), runErr
}

func withCLIStdin(t *testing.T, contents string, fn func() (string, string, error)) (string, string, error) {
	t.Helper()

	stdinPath := filepath.Join(t.TempDir(), "stdin.txt")
	if err := os.WriteFile(stdinPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write stdin file: %v", err)
	}

	stdinFile, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("open stdin file: %v", err)
	}
	defer stdinFile.Close()

	origStdin := os.Stdin
	os.Stdin = stdinFile
	defer func() {
		os.Stdin = origStdin
	}()

	return fn()
}

func writeMCPCLIConfig(t *testing.T) string {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.MCP.Enabled = true
	cfg.MCP.Servers = []config.MCPServerConfig{
		{
			Name:      "helper",
			Command:   os.Args[0],
			Args:      cliHelperArgs("tool-list"),
			Transport: "stdio",
			Enabled:   true,
		},
	}

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath
}

func writeDefaultCLIConfig(t *testing.T) string {
	t.Helper()

	cfg := config.DefaultConfig()
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath
}

func containsListedTool(toolsList []listedMCPTool, source string, name string) bool {
	for _, tool := range toolsList {
		if tool.Source == source && tool.Name == name {
			return true
		}
	}
	return false
}

func cliHelperArgs(mode string) []string {
	args := []string{"-test.run=TestMCPCLIHelperProcess", "--", "helper"}
	if mode != "" {
		args = append(args, mode)
	}
	return args
}

func cliHelperMode() string {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return strings.Join(os.Args[i+1:], " ")
		}
	}
	return ""
}

func cliHelperResponse(req mcp.Request) *mcp.Response {
	switch req.Method {
	case "initialize":
		return &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "helper", "version": "1.0.0"},
			},
		}
	case "notifications/initialized":
		return nil
	case "tools/list":
		return &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo a message",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			},
		}
	case "resources/list":
		return &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"resources": []map[string]any{}},
		}
	case "prompts/list":
		return &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"prompts": []map[string]any{}},
		}
	case "tools/call":
		return &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "tool:ok"},
				},
			},
		}
	default:
		return &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcp.Error{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

func TestCollectMCPToolsWithNilConfigIncludesBuiltins(t *testing.T) {
	toolsList, err := collectMCPTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectMCPTools: %v", err)
	}
	if !containsListedTool(toolsList, "anyclaw", "chat") {
		t.Fatalf("expected builtin tool for nil config, got %#v", toolsList)
	}
}
