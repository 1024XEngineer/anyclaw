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

	"github.com/anyclaw/anyclaw/pkg/pi"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runPiCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printPiUsage()
		return nil
	}

	switch args[0] {
	case "run":
		return runPiServer(ctx, args[1:])
	case "chat":
		return runPiChat(ctx, args[1:])
	case "sessions":
		return runPiSessions(args[1:])
	case "agents":
		return runPiAgents(args[1:])
	case "status":
		return runPiStatus(args[1:])
	default:
		printPiUsage()
		return fmt.Errorf("unknown pi command: %s", args[0])
	}
}

func runPiServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pi run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	if err := fs.Parse(args); err != nil {
		return err
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: *configPath,
		Progress:   bootProgress,
	})
	if err != nil {
		return fmt.Errorf("pi bootstrap failed: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	_ = configPath
	server := pi.NewRPCServer(addr, app.WorkDir, app.Config)

	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printSuccess("Pi Agent RPC listening on %s", addr)
	printInfo("Health: %s/health", addr)
	printInfo("Chat:   %s/v1/chat", addr)

	return server.Start()
}

func runPiChat(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pi chat", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	userID := fs.String("user", "default", "user ID")
	sessionID := fs.String("session", "", "session ID")
	message := fs.String("msg", "", "message to send")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *message == "" {
		return fmt.Errorf("message is required (use --msg)")
	}

	baseURL := fmt.Sprintf("http://%s:%d", *host, *port)
	url := baseURL + "/v1/chat"

	payload := map[string]interface{}{
		"user_id": *userID,
		"input":   *message,
	}
	if *sessionID != "" {
		payload["session_id"] = *sessionID
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %s: %s", resp.Status, body)
	}

	var result pi.RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("error: %s", result.Error)
	}

	data := result.Data.(map[string]interface{})
	printSuccess("Session: %s", data["session_id"])
	fmt.Printf("%s%s%s\n", ui.Bold.Sprint(""), data["response"], ui.Reset.Sprint(""))

	return nil
}

func runPiSessions(args []string) error {
	fs := flag.NewFlagSet("pi sessions", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	userID := fs.String("user", "default", "user ID")
	sessionID := fs.String("session", "", "session ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	baseURL := fmt.Sprintf("http://%s:%d", *host, *port)

	var url string
	if *sessionID != "" {
		url = fmt.Sprintf("%s/v1/sessions/%s?user_id=%s", baseURL, *sessionID, *userID)
	} else {
		url = fmt.Sprintf("%s/v1/sessions?user_id=%s", baseURL, *userID)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %s: %s", resp.Status, body)
	}

	var result pi.RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("error: %s", result.Error)
	}

	sessions := result.Data.([]interface{})
	if len(sessions) == 0 {
		printInfo("No sessions found")
		return nil
	}

	printSuccess("Found %d session(s)", len(sessions))
	for _, s := range sessions {
		session := s.(map[string]interface{})
		fmt.Printf("%s%s%s\n", ui.Bold.Sprint(""), session["id"], ui.Reset.Sprint(""))
		fmt.Printf("  user=%s messages=%d\n", session["user_id"], session["message_count"])
	}

	return nil
}

func runPiAgents(args []string) error {
	fs := flag.NewFlagSet("pi agents", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	userID := fs.String("user", "", "specific user ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	baseURL := fmt.Sprintf("http://%s:%d", *host, *port)

	var url string
	if *userID != "" {
		url = fmt.Sprintf("%s/v1/agents/%s", baseURL, *userID)
	} else {
		url = baseURL + "/v1/agents"
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %s: %s", resp.Status, body)
	}

	var result pi.RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("error: %s", result.Error)
	}

	printSuccess("Pi Agents:")
	if *userID != "" {
		agent := result.Data.(map[string]interface{})
		fmt.Printf("  ID: %s\n", agent["id"])
		fmt.Printf("  Name: %s\n", agent["name"])
		fmt.Printf("  Privacy: %v\n", agent["privacy_mode"])
		fmt.Printf("  Sessions: %v\n", agent["sessions"])
	} else {
		agents := result.Data.([]interface{})
		for _, a := range agents {
			agent := a.(map[string]interface{})
			fmt.Printf("  %s (%s) - %d sessions\n", agent["name"], agent["id"], agent["session_count"])
		}
	}

	return nil
}

func runPiStatus(args []string) error {
	fs := flag.NewFlagSet("pi status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	if err := fs.Parse(args); err != nil {
		return err
	}

	baseURL := fmt.Sprintf("http://%s:%d", *host, *port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/health", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("pi server not reachable at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	var result pi.RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	printSuccess("Pi Agent Server: %s", result.Data.(map[string]interface{})["status"])
	printInfo("Active agents: %d", result.Data.(map[string]interface{})["agents"])
	return nil
}

func printPiUsage() {
	fmt.Print(`AnyClaw Pi Agent commands:

Usage:
  anyclaw pi run [--host 127.0.0.1] [--port 18790]
  anyclaw pi chat --user <user_id> --msg <message>
  anyclaw pi sessions --user <user_id>
  anyclaw pi sessions --user <user_id> --session <session_id>
  anyclaw pi agents
  anyclaw pi agents --user <user_id>
  anyclaw pi status

Examples:
  anyclaw pi run --port 18790
  anyclaw pi chat --user alice --msg "Hello, help me write a function"
  anyclaw pi sessions --user alice
  anyclaw pi agents
`)
}
