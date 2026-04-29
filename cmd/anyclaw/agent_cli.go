package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func runAgentCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printAgentUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list", "ls":
		return runAgentList(args[1:])
	case "use":
		return runAgentUse(args[1:])
	case "chat", "run":
		return runAgentChat(ctx, args[1:])
	default:
		printAgentUsage()
		return fmt.Errorf("unknown agent command: %s", args[0])
	}
}

func runAgentList(args []string) error {
	fs := flag.NewFlagSet("agent list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	type agentView struct {
		Name            string   `json:"name"`
		Description     string   `json:"description,omitempty"`
		Role            string   `json:"role,omitempty"`
		Domain          string   `json:"domain,omitempty"`
		Expertise       []string `json:"expertise,omitempty"`
		WorkingDir      string   `json:"working_dir,omitempty"`
		PermissionLevel string   `json:"permission_level,omitempty"`
		Enabled         bool     `json:"enabled"`
		Current         bool     `json:"current"`
	}

	items := make([]agentView, 0, len(cfg.Agent.Profiles)+1)
	seen := map[string]bool{}
	add := func(item agentView) {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			return
		}
		key := strings.ToLower(item.Name)
		if seen[key] {
			return
		}
		seen[key] = true
		items = append(items, item)
	}

	add(agentView{
		Name:            cfg.ResolveMainAgentName(),
		Description:     cfg.Agent.Description,
		WorkingDir:      cfg.Agent.WorkingDir,
		PermissionLevel: cfg.Agent.PermissionLevel,
		Enabled:         true,
		Current:         true,
	})
	for _, profile := range cfg.Agent.Profiles {
		add(agentView{
			Name:            profile.Name,
			Description:     profile.Description,
			Role:            profile.Role,
			Domain:          profile.Domain,
			Expertise:       append([]string(nil), profile.Expertise...),
			WorkingDir:      profile.WorkingDir,
			PermissionLevel: profile.PermissionLevel,
			Enabled:         profile.IsEnabled(),
			Current:         cfg.IsCurrentAgentProfile(profile.Name),
		})
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"current": cfg.ResolveMainAgentName(),
			"agents":  items,
		})
	}

	fmt.Printf("%s\n\n", ui.Bold.Sprint("Available agents"))
	fmt.Printf("  Current: %s\n\n", cfg.ResolveMainAgentName())
	if len(items) == 0 {
		printInfo("No agents configured")
		return nil
	}
	for _, item := range items {
		status := "enabled"
		if !item.Enabled {
			status = "disabled"
		}
		current := ""
		if item.Current {
			current = " [current]"
		}
		fmt.Printf("  - %s%s (%s)\n", item.Name, current, status)
		if item.Description != "" {
			fmt.Printf("    %s\n", item.Description)
		}
		if item.Domain != "" || len(item.Expertise) > 0 {
			fmt.Printf("    domain=%s expertise=%s\n", item.Domain, strings.Join(item.Expertise, ", "))
		}
	}
	return nil
}

func runAgentUse(args []string) error {
	fs := flag.NewFlagSet("agent use", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	name := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if name == "" {
		return fmt.Errorf("usage: anyclaw agent use <name>")
	}

	cfg, err := config.LoadPersisted(*configPath)
	if err != nil {
		return err
	}
	if !cfg.ApplyAgentProfile(name) {
		return fmt.Errorf("agent not found or disabled: %s", name)
	}
	if err := cfg.Save(*configPath); err != nil {
		return err
	}

	printSuccess("Switched to agent: %s", cfg.ResolveMainAgentName())
	return nil
}

func runAgentChat(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("agent chat", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	agentName := fs.String("agent", "", "agent profile name")
	workingDir := fs.String("cwd", "", "working directory override")
	message := fs.String("message", "", "message to send")
	if err := fs.Parse(args); err != nil {
		return err
	}

	input := strings.TrimSpace(*message)
	if input == "" {
		input = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if input == "" {
		return fmt.Errorf("message is required")
	}

	app, err := appRuntime.NewTargetApp(*configPath, *agentName, *workingDir)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	defer app.Close()

	fmt.Printf("%s %s\n", ui.Bold.Sprint("Agent:"), app.Config.Agent.Name)
	result, err := app.Agent.Run(ctx, input)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}

func printAgentUsage() {
	fmt.Print(`AnyClaw agent commands:

Usage:
  anyclaw agent list [--config anyclaw.json] [--json]
  anyclaw agent use <name> [--config anyclaw.json]
  anyclaw agent chat [--agent <name>] [--message <text>]
  anyclaw agent chat [--agent <name>] <message>
`)
}
