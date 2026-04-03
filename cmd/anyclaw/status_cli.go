package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/gateway"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

type sessionListItem struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Agent             string    `json:"agent,omitempty"`
	MessageCount      int       `json:"message_count"`
	LastUserText      string    `json:"last_user_text,omitempty"`
	LastAssistantText string    `json:"last_assistant_text,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
	LastActiveAt      time.Time `json:"last_active_at,omitempty"`
	Workspace         string    `json:"workspace,omitempty"`
}

type approvalListItem struct {
	ID          string         `json:"id"`
	TaskID      string         `json:"task_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	StepIndex   int            `json:"step_index,omitempty"`
	ToolName    string         `json:"tool_name"`
	Action      string         `json:"action,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	Status      string         `json:"status"`
	RequestedAt time.Time      `json:"requested_at"`
	ResolvedAt  string         `json:"resolved_at,omitempty"`
	ResolvedBy  string         `json:"resolved_by,omitempty"`
	Comment     string         `json:"comment,omitempty"`
}

func runStatusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	deep := fs.Bool("deep", false, "include channel diagnostics")
	all := fs.Bool("all", false, "include channel diagnostics and recent sessions")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadGatewayConfig(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := newGatewayRequestContext()
	defer cancel()

	var status gateway.Status
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/status", nil, &status); err != nil {
		return err
	}

	includeChannels := *deep || *all
	includeSessions := *all

	var channels []channel.Status
	if includeChannels {
		_ = doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/channels", nil, &channels)
	}

	var sessions []sessionListItem
	if includeSessions {
		_ = doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/sessions", nil, &sessions)
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
		})
		if len(sessions) > 5 {
			sessions = sessions[:5]
		}
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"status":   status,
			"channels": channels,
			"sessions": sessions,
		})
	}

	printSuccess("Gateway is %s", status.Status)
	fmt.Printf("Address:  %s\n", status.Address)
	fmt.Printf("Provider: %s\n", status.Provider)
	fmt.Printf("Model:    %s\n", status.Model)
	fmt.Printf("Sessions: %d\n", status.Sessions)
	fmt.Printf("Events:   %d\n", status.Events)
	fmt.Printf("Tools:    %d\n", status.Tools)
	fmt.Printf("Skills:   %d\n", status.Skills)
	fmt.Printf("Workspace:%s %s\n", strings.Repeat(" ", 1), status.WorkingDir)

	if len(channels) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold.Sprint("Channels"))
		printChannelStatusLines(channels, channelStatusPrintOptions{
			DisabledLabel: "stopped",
			ErrorLabel:    "error: ",
		})
	}

	if len(sessions) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold.Sprint("Recent sessions"))
		for _, item := range sessions {
			fmt.Printf("  - %s (%s)\n", item.Title, item.ID)
			fmt.Printf("    agent=%s messages=%d updated=%s\n", item.Agent, item.MessageCount, item.UpdatedAt.Format(time.RFC3339))
		}
	}

	return nil
}

func runHealthCommand(args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	verbose := fs.Bool("verbose", false, "include gateway and channel details")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadGatewayConfig(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := newGatewayRequestContext()
	defer cancel()

	health := map[string]any{}
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/healthz", nil, &health); err != nil {
		return err
	}

	var status gateway.Status
	var channels []channel.Status
	if *verbose {
		_ = doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/status", nil, &status)
		_ = doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/channels", nil, &channels)
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"health":   health,
			"status":   status,
			"channels": channels,
		})
	}

	if ok, _ := health["ok"].(bool); ok {
		printSuccess("Gateway health: ok")
	} else {
		printError("Gateway health check failed")
	}

	if *verbose && status.Address != "" {
		fmt.Printf("Address:  %s\n", status.Address)
		fmt.Printf("Provider: %s\n", status.Provider)
		fmt.Printf("Model:    %s\n", status.Model)
		if len(channels) > 0 {
			healthyCount := 0
			for _, item := range channels {
				if item.Healthy {
					healthyCount++
				}
			}
			fmt.Printf("Channels: %d/%d healthy\n", healthyCount, len(channels))
		}
	}

	return nil
}

func runSessionsCommand(args []string) error {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	activeMinutes := fs.Int("active", 0, "only show sessions active within the last N minutes")
	workspace := fs.String("workspace", "", "filter by workspace")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadGatewayConfig(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := newGatewayRequestContext()
	defer cancel()

	path := "/sessions"
	if strings.TrimSpace(*workspace) != "" {
		path += "?workspace=" + strings.TrimSpace(*workspace)
	}

	var sessions []sessionListItem
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, path, nil, &sessions); err != nil {
		return err
	}

	if *activeMinutes > 0 {
		cutoff := time.Now().Add(-time.Duration(*activeMinutes) * time.Minute)
		filtered := make([]sessionListItem, 0, len(sessions))
		for _, item := range sessions {
			timestamp := item.LastActiveAt
			if timestamp.IsZero() {
				timestamp = item.UpdatedAt
			}
			if timestamp.IsZero() || timestamp.Before(cutoff) {
				continue
			}
			filtered = append(filtered, item)
		}
		sessions = filtered
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"count":    len(sessions),
			"sessions": sessions,
		})
	}

	if len(sessions) == 0 {
		printInfo("No sessions found")
		return nil
	}

	printSuccess("Found %d session(s)", len(sessions))
	for _, item := range sessions {
		fmt.Printf("%s%s%s\n", ui.Bold.Sprint(""), item.Title, ui.Reset.Sprint(""))
		fmt.Printf("  id=%s agent=%s messages=%d updated=%s\n", item.ID, item.Agent, item.MessageCount, item.UpdatedAt.Format(time.RFC3339))
		if strings.TrimSpace(item.Workspace) != "" {
			fmt.Printf("  workspace=%s\n", item.Workspace)
		}
		if strings.TrimSpace(item.LastUserText) != "" {
			fmt.Printf("  last_user=%s\n", item.LastUserText)
		}
	}

	return nil
}

func runApprovalsCommand(args []string) error {
	subcommand := "get"
	if len(args) > 0 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		subcommand = strings.ToLower(strings.TrimSpace(args[0]))
		args = args[1:]
	}

	switch subcommand {
	case "get", "list":
		return runApprovalsGet(args)
	case "approve":
		return runApprovalsResolve(args, true)
	case "reject":
		return runApprovalsResolve(args, false)
	default:
		printApprovalsUsage()
		return fmt.Errorf("unknown approvals command: %s", subcommand)
	}
}

func runApprovalsGet(args []string) error {
	fs := flag.NewFlagSet("approvals get", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	status := fs.String("status", "pending", "filter by approval status")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadGatewayConfig(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := newGatewayRequestContext()
	defer cancel()

	path := "/approvals"
	if trimmed := strings.TrimSpace(*status); trimmed != "" {
		path += "?status=" + trimmed
	}

	var approvals []approvalListItem
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, path, nil, &approvals); err != nil {
		return err
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"count":     len(approvals),
			"approvals": approvals,
		})
	}

	if len(approvals) == 0 {
		printInfo("No approvals found")
		return nil
	}

	printSuccess("Found %d approval(s)", len(approvals))
	for _, item := range approvals {
		fmt.Printf("%s%s%s\n", ui.Bold.Sprint(""), item.ID, ui.Reset.Sprint(""))
		fmt.Printf("  status=%s tool=%s action=%s requested=%s\n", item.Status, item.ToolName, item.Action, item.RequestedAt.Format(time.RFC3339))
		if strings.TrimSpace(item.TaskID) != "" {
			fmt.Printf("  task=%s\n", item.TaskID)
		}
		if strings.TrimSpace(item.SessionID) != "" {
			fmt.Printf("  session=%s\n", item.SessionID)
		}
	}

	return nil
}

func runApprovalsResolve(args []string, approved bool) error {
	name := "approvals approve"
	if !approved {
		name = "approvals reject"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	comment := fs.String("comment", "", "optional resolution comment")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("approval id is required")
	}
	approvalID := strings.TrimSpace(fs.Arg(0))
	if approvalID == "" {
		return fmt.Errorf("approval id is required")
	}

	cfg, err := loadGatewayConfig(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := newGatewayRequestContext()
	defer cancel()

	var updated approvalListItem
	if err := doGatewayJSONRequest(ctx, cfg, http.MethodPost, "/approvals/"+approvalID+"/resolve", map[string]any{
		"approved": approved,
		"comment":  strings.TrimSpace(*comment),
	}, &updated); err != nil {
		return err
	}

	if approved {
		printSuccess("Approved: %s", updated.ID)
	} else {
		printSuccess("Rejected: %s", updated.ID)
	}
	return nil
}

func printApprovalsUsage() {
	fmt.Print(`AnyClaw approvals commands:

Usage:
  anyclaw approvals get [--status pending]
  anyclaw approvals approve <id> [--comment "looks good"]
  anyclaw approvals reject <id> [--comment "stop"]
`)
}

func writePrettyJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
