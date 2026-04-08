package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/consoleio"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runAgentCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printAgentUsage()
		return nil
	}

	switch args[0] {
	case "chat":
		return runAgentChat(ctx, args[1:])
	case "list":
		return runAgentList()
	case "use":
		return runAgentUse(args[1:])
	default:
		printAgentUsage()
		return fmt.Errorf("unknown agent command: %s", args[0])
	}
}

func printAgentUsage() {
	fmt.Print(`AnyClaw agent commands:

Usage:
  anyclaw agent list
  anyclaw agent use <name>
  anyclaw agent chat [name]
  anyclaw agent chat --agent <name>
`)
}

func runAgentList() error {
	cfg, err := config.Load("anyclaw.json")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("%s\n\n", ui.Bold.Sprint("Available agents"))
	fmt.Printf("  %sCurrent: %s%s\n\n", ui.Dim.Sprint(""), cfg.ResolveMainAgentName(), ui.Reset.Sprint(""))

	if len(cfg.Agent.Profiles) == 0 {
		fmt.Println("  (no agent profiles configured in anyclaw.json)")
		return nil
	}

	for _, p := range cfg.Agent.Profiles {
		status := ui.Dim.Sprint("disabled")
		if p.IsEnabled() {
			status = ui.Green.Sprint("enabled")
		}
		fmt.Printf("  %s %s\n", status, ui.Bold.Sprint(p.Name))
		if p.Description != "" {
			fmt.Printf("     %s\n", ui.Dim.Sprint(p.Description))
		}
		if p.Domain != "" {
			fmt.Printf("     domain: %s", p.Domain)
		}
		if len(p.Expertise) > 0 {
			fmt.Printf(" | expertise: %s", strings.Join(p.Expertise, ", "))
		}
		if p.Domain != "" || len(p.Expertise) > 0 {
			fmt.Println()
		}
		fmt.Println()
	}
	return nil
}

func runAgentUse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw agent use <name>")
	}
	name := strings.Join(args, " ")

	cfg, err := config.Load("anyclaw.json")
	if err != nil {
		return err
	}

	if !cfg.ApplyAgentProfile(name) {
		fmt.Fprintf(os.Stderr, "agent not found: %s\n\nAvailable agents:\n", name)
		for _, p := range cfg.Agent.Profiles {
			fmt.Fprintf(os.Stderr, "  - %s\n", p.Name)
		}
		return fmt.Errorf("agent not found: %s", name)
	}

	if err := cfg.Save("anyclaw.json"); err != nil {
		return err
	}
	printSuccess("Switched to agent: %s", name)
	return nil
}

func runAgentChat(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("agent chat", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	agentName := fs.String("agent", "", "agent name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *agentName == "" && fs.NArg() > 0 {
		*agentName = strings.Join(fs.Args(), " ")
	}

	cfg, err := config.Load("anyclaw.json")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if *agentName == "" {
		*agentName = cfg.ResolveMainAgentName()
	} else if profile, ok := cfg.ResolveAgentProfile(*agentName); ok {
		*agentName = profile.Name
	} else if !strings.EqualFold(strings.TrimSpace(*agentName), cfg.ResolveMainAgentName()) {
		if profile, ok := cfg.FindAgentProfile(*agentName); ok && !profile.IsEnabled() {
			return fmt.Errorf("agent is disabled: %s", *agentName)
		}
		return fmt.Errorf("agent not found: %s", *agentName)
	}

	app, err := appRuntime.NewTargetApp("anyclaw.json", *agentName, "")
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	state := &RuntimeState{
		llmClient:  app.LLM,
		cfg:        app.Config,
		agent:      app.Agent,
		skills:     app.Skills,
		audit:      app.Audit,
		reader:     consoleio.NewReader(os.Stdin),
		configPath: app.ConfigPath,
		workDir:    app.WorkDir,
		workingDir: app.WorkingDir,
		gatewayURL: appRuntime.GatewayURL(app.Config),
	}

	lines := []string{
		ui.KeyValue("agent", state.cfg.Agent.Name),
		ui.KeyValue("model", fmt.Sprintf("%s / %s", state.cfg.LLM.Provider, state.cfg.LLM.Model)),
	}
	if state.workingDir != "" {
		lines = append(lines, ui.KeyValue("dir", state.workingDir))
	}
	if state.gatewayURL != "" {
		lines = append(lines, ui.KeyValue("gateway", state.gatewayURL+" (configured)"))
	}
	lines = append(lines, ui.KeyValue("output", interactiveOutputMode(state)))
	printInteractiveHeaderWithHelp("Agent Chat", nil, lines...)

	for {
		input, err := readInteractiveLineStable(state)
		if err != nil {
			break
		}
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			if handleAgentChatCommand(ctx, state, input) {
				break
			}
			continue
		}

		fmt.Println()
		routeLabel := applyLLMRouteStable(state, input)
		resp, err := state.agent.Run(ctx, input)
		if err != nil {
			printError("%v", err)
			continue
		}
		if routeLabel != "" {
			fmt.Printf("%s%s%s\n", ui.Dim.Sprint(""), routeLabel, ui.Reset.Sprint(""))
		}
		printAssistantResponse(state, resp)
	}
	return nil
}

func printAgentChatHelp() {
	printInteractiveHelpSections("  /gateway             - show configured gateway address")
}

func handleAgentChatCommand(ctx context.Context, state *RuntimeState, input string) bool {
	commandText := strings.ToLower(strings.TrimSpace(input))
	switch commandText {
	case "/help", "/?":
		fmt.Println()
		printAgentChatHelp()
		return false
	}
	return handleCommandStable(ctx, state, input)
}
