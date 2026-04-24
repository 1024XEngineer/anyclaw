package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/mcp"
)

type listedMCPTool struct {
	Source      string `json:"source"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

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

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	srv := newBuiltinMCPServer(defaultMCPServerName(cfg))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
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

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	toolsList, listErr := collectMCPTools(ctx, cfg)
	if *jsonOut {
		data, _ := json.MarshalIndent(toolsList, "", "  ")
		fmt.Println(string(data))
		return listErr
	}

	fmt.Printf("Available MCP tools (%d):\n\n", len(toolsList))
	for _, t := range toolsList {
		fmt.Printf("  [%s] %s\n    %s\n\n", t.Source, t.Name, t.Description)
	}
	return listErr
}

func collectMCPTools(ctx context.Context, cfg *config.Config) ([]listedMCPTool, error) {
	srv := newBuiltinMCPServer(defaultMCPServerName(cfg))
	toolsList := make([]listedMCPTool, 0, len(srv.ListTools()))
	for _, tool := range srv.ListTools() {
		toolsList = append(toolsList, listedMCPTool{
			Source:      defaultMCPServerName(cfg),
			Name:        tool.Name,
			Description: tool.Description,
		})
	}

	if cfg == nil || !cfg.MCP.Enabled || len(cfg.MCP.Servers) == 0 {
		sortListedMCPTools(toolsList)
		return toolsList, nil
	}

	registry := mcp.NewRegistry()
	var errs []error
	for _, serverCfg := range cfg.MCP.Servers {
		if !serverCfg.Enabled || strings.TrimSpace(serverCfg.Command) == "" {
			continue
		}
		name := strings.TrimSpace(serverCfg.Name)
		if name == "" {
			name = serverCfg.Command
		}
		client := mcp.NewClient(name, serverCfg.Command, serverCfg.Args, serverCfg.Env)
		if err := registry.Register(name, client); err != nil {
			errs = append(errs, fmt.Errorf("register MCP %s: %w", name, err))
		}
	}
	defer registry.DisconnectAll()

	errs = append(errs, registry.ConnectAll(ctx)...)
	for source, remoteTools := range registry.AllTools() {
		for _, tool := range remoteTools {
			toolsList = append(toolsList, listedMCPTool{
				Source:      source,
				Name:        tool.Name,
				Description: tool.Description,
			})
		}
	}

	sortListedMCPTools(toolsList)
	if len(errs) == 0 {
		return toolsList, nil
	}
	return toolsList, errors.Join(errs...)
}

func newBuiltinMCPServer(name string) *mcp.Server {
	srv := mcp.NewServer(name, "1.0.0")
	registerBuiltinMCPTools(srv)
	return srv
}

func defaultMCPServerName(cfg *config.Config) string {
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Agent.Name); name != "" {
			return name
		}
	}
	return "anyclaw"
}

func sortListedMCPTools(toolsList []listedMCPTool) {
	sort.Slice(toolsList, func(i, j int) bool {
		if toolsList[i].Source != toolsList[j].Source {
			return toolsList[i].Source < toolsList[j].Source
		}
		return toolsList[i].Name < toolsList[j].Name
	})
}

func registerBuiltinMCPTools(srv *mcp.Server) {
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
