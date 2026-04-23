package gateway

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/mcp"
)

func (s *Server) initMCP(ctx context.Context) {
	s.mcpRegistry = mcp.NewRegistry()

	cfg := s.mainRuntime.Config.MCP
	if !cfg.Enabled || len(cfg.Servers) == 0 {
		return
	}

	for _, srvCfg := range cfg.Servers {
		if !srvCfg.Enabled || srvCfg.Command == "" {
			continue
		}
		client := mcp.NewClient(srvCfg.Name, srvCfg.Command, srvCfg.Args, srvCfg.Env)
		if err := s.mcpRegistry.Register(srvCfg.Name, client); err != nil {
			fmt.Fprintf(os.Stderr, "MCP register %s: %v\n", srvCfg.Name, err)
			continue
		}
	}

	if errs := s.mcpRegistry.ConnectAll(ctx); len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "MCP connect: %v\n", err)
		}
	}

	if s.mcpRegistry != nil {
		if err := mcp.BridgeToToolRegistry(s.mainRuntime.ToolRegistry(), s.mcpRegistry); err != nil {
			fmt.Fprintf(os.Stderr, "MCP bridge: %v\n", err)
		}
	}

	s.mcpServer = mcp.NewServer("anyclaw", "1.0.0")
	s.registerBuiltinMCPTools()
}

func (s *Server) registerBuiltinMCPTools() {
	s.mcpServer.RegisterTool(mcp.ServerTool{
		Name:        "chat",
		Description: "Send a message to AnyClaw AI agent",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string", "description": "User message"},
			},
			"required": []string{"message"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			msg, _ := args["message"].(string)
			if msg == "" {
				return nil, fmt.Errorf("message is required")
			}
			return "Chat endpoint available via HTTP POST /chat", nil
		},
	})

	s.mcpServer.RegisterResource(mcp.ServerResource{
		URI:         "status://gateway",
		Name:        "Gateway Status",
		Description: "Current gateway server status",
		MimeType:    "application/json",
		Handler: func(ctx context.Context) (any, error) {
			return map[string]any{
				"started_at": s.startedAt,
				"uptime":     time.Since(s.startedAt).String(),
			}, nil
		},
	})

	s.mcpServer.RegisterPrompt(mcp.ServerPrompt{
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
}
