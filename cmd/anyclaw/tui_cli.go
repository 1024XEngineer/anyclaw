package main

import (
	"fmt"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/tui"
)

func runTUICommand(args []string) error {
	cfg := loadConfigForTUI()

	gatewayURL := fmt.Sprintf("http://%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	token := cfg.Security.APIToken

	client := tui.NewGatewayClient(gatewayURL, token)

	if len(args) > 0 && args[0] == "--help" {
		printTUIHelp()
		return nil
	}

	// Check gateway health first
	if err := client.CheckHealth(); err != nil {
		return fmt.Errorf("gateway not reachable at %s: %w\n\nStart the gateway with: anyclaw gateway start", gatewayURL, err)
	}

	// Check for session ID argument
	var sessionID string
	for i, arg := range args {
		if arg == "--session" && i+1 < len(args) {
			sessionID = args[i+1]
			break
		}
	}

	if sessionID != "" {
		return tui.RunWithSession(client, sessionID)
	}

	return tui.Run(client)
}

func printTUIHelp() {
	fmt.Println(`AnyClaw TUI - Terminal User Interface

Usage:
  anyclaw tui [flags]

Flags:
  --session <id>   Open a specific session
  --help           Show this help

Keyboard Shortcuts:
  ctrl+c / esc     Quit
  ?                Toggle help
  1 / c            Switch to Chat panel
  2 / s            Switch to Sessions panel
  3 / t            Switch to Status panel
  ctrl+d           Send message
  ctrl+n           New session
  ctrl+l           Clear chat
  enter            Select session (in Sessions panel)
  d                Delete session (in Sessions panel)
  up/down          Navigate list
  tab              Next panel
  shift+tab        Previous panel`)
}

func loadConfigForTUI() *config.Config {
	cfg, err := config.Load("")
	if err != nil {
		cfg = config.DefaultConfig()
	}
	return cfg
}
