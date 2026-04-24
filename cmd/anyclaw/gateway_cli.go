package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/gateway"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

var bootstrapGatewayRuntime = appRuntime.Bootstrap

var runGatewayRuntime = func(ctx context.Context, app *appRuntime.MainRuntime) error {
	if app.Config.Gateway.WorkerCount > 1 {
		return gateway.RunWithWorkers(ctx, app)
	}
	return gateway.New(app).Run(ctx)
}

var startGatewayDaemon = gateway.StartDetached
var stopGatewayDaemon = gateway.StopDetached

func runGatewayCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printGatewayUsage()
		return nil
	}

	switch normalizeGatewayCommand(args[0]) {
	case "run":
		return runGatewayServer(ctx, args[1:])
	case "daemon":
		return runGatewayDaemon(args[1:])
	case "status":
		return runGatewayStatus(args[1:])
	case "sessions":
		return runGatewaySessions(args[1:])
	case "events":
		return runGatewayEvents(args[1:])
	default:
		printGatewayUsage()
		return fmt.Errorf("unknown gateway command: %s", args[0])
	}
}

func normalizeGatewayCommand(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "start":
		return "run"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func runGatewayServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gateway run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	host := fs.String("host", "", "gateway host")
	port := fs.Int("port", 0, "gateway port")
	workers := fs.Int("workers", 0, "number of worker processes (0=auto)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := ensureGatewayControlUIBuilt(ctx, *configPath); err != nil {
		return err
	}

	app, err := bootstrapGatewayRuntime(appRuntime.BootstrapOptions{
		ConfigPath: *configPath,
	})
	if err != nil {
		return fmt.Errorf("gateway bootstrap failed: %w", err)
	}
	if *host != "" {
		app.Config.Gateway.Host = *host
	}
	if *port > 0 {
		app.Config.Gateway.Port = *port
	}
	if *workers > 0 {
		app.Config.Gateway.WorkerCount = *workers
	}

	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printInfo("Gateway workers: %d", app.Config.Gateway.WorkerCount)
	printSuccess("Gateway listening on %s", appRuntime.GatewayAddress(app.Config))
	printInfo("Health: %s/healthz", gatewayHTTPBaseURL(app.Config))
	printInfo("Status: %s/status", gatewayHTTPBaseURL(app.Config))

	return runGatewayRuntime(ctx, app)
}

func runGatewayDaemon(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw gateway daemon <start|stop>")
	}

	action := strings.TrimSpace(args[0])
	fs := flag.NewFlagSet("gateway daemon "+action, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: anyclaw gateway daemon <start|stop> [--config <path>]")
	}

	if action == "start" {
		if err := ensureGatewayControlUIBuilt(context.Background(), *configPath); err != nil {
			return err
		}
	}
	app, err := bootstrapGatewayRuntime(appRuntime.BootstrapOptions{
		ConfigPath: *configPath,
	})
	if err != nil {
		return fmt.Errorf("daemon bootstrap failed: %w", err)
	}
	app.ConfigPath = *configPath

	switch action {
	case "start":
		if err := startGatewayDaemon(app); err != nil {
			return err
		}
		printSuccess("Gateway daemon started")
		return nil
	case "stop":
		if err := stopGatewayDaemon(app); err != nil {
			return err
		}
		printSuccess("Gateway daemon stopped")
		return nil
	default:
		return fmt.Errorf("unknown daemon command: %s", action)
	}
}

func runGatewayStatus(args []string) error {
	fs := flag.NewFlagSet("gateway status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var status gateway.Status
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/status", nil, &status); err != nil {
		return fmt.Errorf("gateway not reachable at %s: %w", gatewayHTTPBaseURL(cfg), err)
	}

	printSuccess("Gateway is %s", status.Status)
	fmt.Printf("%sAddress: %s\n", ui.Cyan.Sprint(""), status.Address)
	fmt.Printf("%sProvider: %s\n", ui.Cyan.Sprint(""), status.Provider)
	fmt.Printf("%sModel: %s\n", ui.Cyan.Sprint(""), status.Model)
	fmt.Printf("%sSessions: %d\n", ui.Cyan.Sprint(""), status.Sessions)
	fmt.Printf("%sEvents: %d\n", ui.Cyan.Sprint(""), status.Events)
	fmt.Printf("%sTools: %d\n", ui.Cyan.Sprint(""), status.Tools)
	fmt.Printf("%sSkills: %d\n", ui.Cyan.Sprint(""), status.Skills)
	return nil
}

func runGatewaySessions(args []string) error {
	fs := flag.NewFlagSet("gateway sessions", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var sessions []struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		MessageCount int    `json:"message_count"`
		UpdatedAt    string `json:"updated_at"`
	}
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/sessions", nil, &sessions); err != nil {
		return err
	}

	if len(sessions) == 0 {
		printInfo("No gateway sessions yet")
		return nil
	}

	printSuccess("Found %d gateway session(s)", len(sessions))
	for _, session := range sessions {
		fmt.Printf("%s%s%s\n", ui.Bold.Sprint(""), session.Title, ui.Reset.Sprint(""))
		fmt.Printf("  id=%s messages=%d updated=%s\n", session.ID, session.MessageCount, session.UpdatedAt)
	}
	return nil
}

func runGatewayEvents(args []string) error {
	fs := flag.NewFlagSet("gateway events", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	stream := fs.Bool("stream", false, "stream events over SSE")
	replay := fs.Int("replay", 10, "number of recent events to replay for stream mode")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	baseURL := gatewayHTTPBaseURL(cfg)
	if *stream {
		url := fmt.Sprintf("%s/events/stream?replay=%d", baseURL, *replay)
		printInfo("Streaming events from %s", url)
		req, err := newGatewayRequest(context.Background(), cfg, httpMethodGet, fmt.Sprintf("/events/stream?replay=%d", *replay), nil)
		if err != nil {
			return err
		}
		resp, err := gatewayHTTPClient(30 * time.Second).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("gateway returned %s", resp.Status)
		}
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var events []struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		Timestamp string `json:"timestamp"`
	}
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/events", nil, &events); err != nil {
		return err
	}

	if len(events) == 0 {
		printInfo("No gateway events yet")
		return nil
	}

	printSuccess("Found %d gateway event(s)", len(events))
	for _, event := range events {
		fmt.Printf("- %s session=%s at %s id=%s\n", event.Type, event.SessionID, event.Timestamp, event.ID)
	}
	return nil
}

const httpMethodGet = "GET"

func printGatewayUsage() {
	fmt.Print(`AnyClaw gateway commands:

Usage:
  anyclaw gateway start [--host 127.0.0.1] [--port 18789]
  anyclaw gateway run [--host 127.0.0.1] [--port 18789]
  anyclaw gateway daemon start
  anyclaw gateway daemon stop
  anyclaw gateway status
  anyclaw gateway sessions
  anyclaw gateway events
  anyclaw gateway events --stream
`)
}
