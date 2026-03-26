package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/gateway"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runGatewayCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printGatewayUsage()
		return nil
	}

	switch args[0] {
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

func runGatewayServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gateway run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	host := fs.String("host", "", "gateway host")
	port := fs.Int("port", 0, "gateway port")
	if err := fs.Parse(args); err != nil {
		return err
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: *configPath,
		Progress:   bootProgress,
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

	server := gateway.New(app)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printSuccess("Gateway listening on %s", appRuntime.GatewayAddress(app.Config))
	printInfo("Health: %s/healthz", appRuntime.GatewayURL(app.Config))
	printInfo("Status: %s/status", appRuntime.GatewayURL(app.Config))
	return server.Run(ctx)
}

func runGatewayDaemon(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw gateway daemon <start|stop>")
	}
	configPath := "anyclaw.json"
	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: configPath,
		Progress:   bootProgress,
	})
	if err != nil {
		return fmt.Errorf("daemon bootstrap failed: %w", err)
	}
	app.ConfigPath = configPath

	switch args[0] {
	case "start":
		if err := gateway.StartDetached(app); err != nil {
			return err
		}
		printSuccess("Gateway daemon started")
		return nil
	case "stop":
		if err := gateway.StopDetached(app); err != nil {
			return err
		}
		printSuccess("Gateway daemon stopped")
		return nil
	default:
		return fmt.Errorf("unknown daemon command: %s", args[0])
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
	status, err := gateway.Probe(ctx, appRuntime.GatewayURL(cfg))
	if err != nil {
		return fmt.Errorf("gateway not reachable at %s: %w", appRuntime.GatewayURL(cfg), err)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appRuntime.GatewayURL(cfg)+"/sessions", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %s", resp.Status)
	}

	var sessions []struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		MessageCount int    `json:"message_count"`
		UpdatedAt    string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
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

	baseURL := appRuntime.GatewayURL(cfg)
	if *stream {
		url := fmt.Sprintf("%s/events/stream?replay=%d", baseURL, *replay)
		printInfo("Streaming events from %s", url)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("gateway returned %s", resp.Status)
		}
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/events", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %s", resp.Status)
	}

	var events []struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
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

func printGatewayUsage() {
	fmt.Print(`AnyClaw gateway commands:

Usage:
  anyclaw gateway run [--host 127.0.0.1] [--port 18789]
  anyclaw gateway daemon start
  anyclaw gateway daemon stop
  anyclaw gateway status
  anyclaw gateway sessions
  anyclaw gateway events
  anyclaw gateway events --stream
`)
}
