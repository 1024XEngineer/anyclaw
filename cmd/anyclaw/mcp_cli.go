package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/mcp"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

func runMCPCommand(args []string) error {
	if len(args) == 0 {
		printMCPUsage()
		return nil
	}
	switch args[0] {
	case "serve":
		return runMCPServe(args[1:])
	case "tools":
		return runMCPTools(args[1:])
	default:
		printMCPUsage()
		return fmt.Errorf("unknown mcp command: %s", args[0])
	}
}

func runMCPServe(args []string) error {
	fs := flag.NewFlagSet("mcp serve", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	srv := mcp.NewServer("anyclaw", "1.0.0")

	registry := tools.NewRegistry()
	registerBuiltinMCPTools(registry, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	fmt.Fprintf(os.Stderr, "mcp server: anyclaw MCP server started (stdio)\n")
	return srv.Run(ctx)
}

func runMCPTools(args []string) error {
	fs := flag.NewFlagSet("mcp tools", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	registry := tools.NewRegistry()
	srv := mcp.NewServer("anyclaw", "1.0.0")
	registerBuiltinMCPTools(registry, srv)

	toolsList := srv.ListTools()
	if *jsonOut {
		data, _ := json.MarshalIndent(toolsList, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Available MCP tools (%d):\n\n", len(toolsList))
	for _, t := range toolsList {
		fmt.Printf("  %s\n    %s\n\n", t.Name, t.Description)
	}
	return nil
}

func registerBuiltinMCPTools(registry *tools.Registry, srv *mcp.Server) {
	srv.RegisterTool(mcp.ServerTool{
		Name:        "chat",
		Description: "Send a message to AnyClaw AI agent and get a response",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string", "description": "User message to send to the agent"},
			},
			"required": []string{"message"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			msg, _ := args["message"].(string)
			if msg == "" {
				return nil, fmt.Errorf("message is required")
			}
			return fmt.Sprintf("Message received: %s\n\nNote: Direct agent chat requires the gateway to be running.", msg), nil
		},
	})

	srv.RegisterTool(mcp.ServerTool{
		Name:        "list_sessions",
		Description: "List active sessions",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return "Session listing requires the gateway to be running.", nil
		},
	})

	srv.RegisterTool(mcp.ServerTool{
		Name:        "list_agents",
		Description: "List available agents",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return "Agent listing requires the gateway to be running.", nil
		},
	})

	srv.RegisterResource(mcp.ServerResource{
		URI:         "status://gateway",
		Name:        "Gateway Status",
		Description: "Current gateway server status and health",
		MimeType:    "application/json",
		Handler: func(ctx context.Context) (any, error) {
			return map[string]any{
				"status":  "mcp_server_only",
				"message": "Full gateway status requires the gateway to be running",
			}, nil
		},
	})

	srv.RegisterPrompt(mcp.ServerPrompt{
		Name:        "code_review",
		Description: "Code review prompt template",
		Arguments: []mcp.PromptArg{
			{Name: "language", Description: "Programming language", Required: false},
			{Name: "focus", Description: "Review focus (security, performance, style)", Required: false},
		},
		Handler: func(ctx context.Context, args map[string]string) ([]mcp.PromptMessage, error) {
			lang := args["language"]
			focus := args["focus"]
			text := "Please review the following code"
			if lang != "" {
				text += " (" + lang + ")"
			}
			if focus != "" {
				text += " with focus on " + focus
			}
			text += ":\n\n"
			return []mcp.PromptMessage{
				{Role: "user", Content: struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: text}},
			}, nil
		},
	})

	srv.RegisterPrompt(mcp.ServerPrompt{
		Name:        "explain_code",
		Description: "Explain code prompt template",
		Arguments: []mcp.PromptArg{
			{Name: "level", Description: "Explanation level (beginner, intermediate, expert)", Required: false},
		},
		Handler: func(ctx context.Context, args map[string]string) ([]mcp.PromptMessage, error) {
			level := args["level"]
			text := "Please explain the following code"
			if level != "" {
				text += " at a " + level + " level"
			}
			text += ":\n\n"
			return []mcp.PromptMessage{
				{Role: "user", Content: struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: text}},
			}, nil
		},
	})
}

func printMCPUsage() {
	fmt.Print(`AnyClaw MCP commands:
Usage:
  anyclaw mcp serve [--config <path>]    Run as MCP server (stdio)
  anyclaw mcp tools [--config <path>]    List available MCP tools
`)
}
