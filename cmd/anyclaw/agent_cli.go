package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/chat"
	"github.com/anyclaw/anyclaw/pkg/config"
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
	fmt.Printf("  %sCurrent: %s%s\n\n", ui.Dim.Sprint(""), cfg.Agent.Name, ui.Reset.Sprint(""))

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

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: "anyclaw.json",
		Progress:   func(ev appRuntime.BootEvent) {},
	})
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	if app.Orchestrator == nil {
		return fmt.Errorf("orchestrator is disabled; set orchestrator.enabled=true in anyclaw.json")
	}

	agents := app.Orchestrator.ListAgents()
	if len(agents) == 0 {
		return fmt.Errorf("no agents available")
	}

	if *agentName == "" {
		fmt.Printf("%s\n\n", ui.Bold.Sprint("Choose an agent"))
		for i, a := range agents {
			domain := ""
			if a.Domain != "" {
				domain = " [" + a.Domain + "]"
			}
			fmt.Printf("  %s %s%s\n", ui.Cyan.Sprint(fmt.Sprintf("%d.", i+1)), ui.Bold.Sprint(a.Name), ui.Dim.Sprint(domain))
			fmt.Printf("     %s\n\n", a.Description)
		}
		fmt.Printf("%sEnter agent number or name: %s", ui.Green.Sprint(""), ui.Reset.Sprint(""))
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(agents) {
			*agentName = agents[idx-1].Name
		} else {
			*agentName = input
		}
	}

	found := false
	for _, a := range agents {
		if a.Name == *agentName {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("agent not found: %s", *agentName)
	}

	chatMgr := chat.NewChatManager(app.Orchestrator)
	reader := bufio.NewReader(os.Stdin)
	var sessionID string

	fmt.Println()
	printSuccess("Chatting with [%s] (/exit to quit, /clear to reset)", *agentName)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	fmt.Println()

	for {
		fmt.Printf("%s%s > %s", ui.Dim.Sprint("["), ui.Bold.Sprint(*agentName), ui.Reset.Sprint(""))
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "/exit" || input == "/quit" {
			printSuccess("Bye")
			break
		}
		if input == "/clear" {
			sessionID = ""
			printSuccess("Chat history cleared")
			continue
		}

		resp, err := chatMgr.Chat(ctx, chat.ChatRequest{
			AgentName: *agentName,
			SessionID: sessionID,
			Message:   input,
		})
		if err != nil {
			printError("%v", err)
			continue
		}
		sessionID = resp.SessionID
		fmt.Printf("\n%s\n\n", ui.Bold.Sprint(resp.Message.Content))
	}
	return nil
}
