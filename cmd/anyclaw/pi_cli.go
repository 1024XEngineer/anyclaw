package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/pi"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func runPiCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printPiUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "run", "start":
		return runPiServer(ctx, args[1:])
	case "chat":
		return runPiChat(ctx, args[1:])
	case "sessions":
		return runPiSessions(args[1:])
	case "agents":
		return runPiAgents(args[1:])
	case "status", "health":
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

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{ConfigPath: *configPath})
	if err != nil {
		return fmt.Errorf("pi bootstrap failed: %w", err)
	}
	defer app.Close()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	server := pi.NewRPCServer(addr, app.WorkDir, app.Config)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printSuccess("Pi Agent RPC listening on %s", addr)
	printInfo("Health: http://%s/v1/health", addr)
	printInfo("Chat:   http://%s/v1/chat", addr)

	select {
	case <-ctx.Done():
		_ = server.Stop()
		return nil
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
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

	input := strings.TrimSpace(*message)
	if input == "" {
		input = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if input == "" {
		return fmt.Errorf("message is required")
	}

	var result pi.RPCResponse
	if err := doPiJSON(ctx, *host, *port, http.MethodPost, "/v1/chat", map[string]any{
		"user_id":    strings.TrimSpace(*userID),
		"session_id": strings.TrimSpace(*sessionID),
		"input":      input,
	}, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("pi chat failed: %s", result.Error)
	}

	data, _ := result.Data.(map[string]any)
	if session := strings.TrimSpace(fmt.Sprint(data["session_id"])); session != "" && session != "<nil>" {
		printSuccess("Session: %s", session)
	}
	fmt.Println(fmt.Sprint(data["response"]))
	return nil
}

func runPiSessions(args []string) error {
	fs := flag.NewFlagSet("pi sessions", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	userID := fs.String("user", "default", "user ID")
	sessionID := fs.String("session", "", "session ID")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := buildPiSessionsPath(*userID, *sessionID)

	var result pi.RPCResponse
	if err := doPiJSON(context.Background(), *host, *port, http.MethodGet, path, nil, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("pi sessions failed: %s", result.Error)
	}
	if *jsonOut {
		return writePrettyJSON(result.Data)
	}
	return printPiDataList("sessions", result.Data)
}

func runPiAgents(args []string) error {
	fs := flag.NewFlagSet("pi agents", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	userID := fs.String("user", "", "specific user ID")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := buildPiAgentsPath(*userID)

	var result pi.RPCResponse
	if err := doPiJSON(context.Background(), *host, *port, http.MethodGet, path, nil, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("pi agents failed: %s", result.Error)
	}
	if *jsonOut {
		return writePrettyJSON(result.Data)
	}
	return printPiDataList("agents", result.Data)
}

func runPiStatus(args []string) error {
	fs := flag.NewFlagSet("pi status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	host := fs.String("host", "127.0.0.1", "RPC server host")
	port := fs.Int("port", 18790, "RPC server port")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result pi.RPCResponse
	if err := doPiJSON(ctx, *host, *port, http.MethodGet, "/v1/health", nil, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("pi status failed: %s", result.Error)
	}
	data, _ := result.Data.(map[string]any)
	printSuccess("Pi Agent Server: %v", data["status"])
	printInfo("Active agents: %v", data["agents"])
	return nil
}

func buildPiSessionsPath(userID string, sessionID string) string {
	path := "/v1/sessions"
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		path += "/" + url.PathEscape(sessionID)
	}

	query := url.Values{}
	query.Set("user_id", strings.TrimSpace(userID))
	return path + "?" + query.Encode()
}

func buildPiAgentsPath(userID string) string {
	path := "/v1/agents"
	if userID = strings.TrimSpace(userID); userID != "" {
		path += "/" + url.PathEscape(userID)
	}
	return path
}

func doPiJSON(ctx context.Context, host string, port int, method string, path string, body any, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	baseURL := fmt.Sprintf("http://%s:%d", strings.TrimSpace(host), port)
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("pi server not reachable at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pi server returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func printPiDataList(label string, data any) error {
	switch typed := data.(type) {
	case []any:
		if len(typed) == 0 {
			printInfo("No %s found", label)
			return nil
		}
		printSuccess("Found %d %s", len(typed), label)
		for _, item := range typed {
			fmt.Printf("  - %v\n", item)
		}
	case map[string]any:
		for key, value := range typed {
			fmt.Printf("%s: %v\n", key, value)
		}
	default:
		fmt.Println(data)
	}
	return nil
}

func printPiUsage() {
	fmt.Print(`AnyClaw Pi Agent commands:

Usage:
  anyclaw pi run [--config anyclaw.json] [--host 127.0.0.1] [--port 18790]
  anyclaw pi chat --user <user_id> --msg <message>
  anyclaw pi sessions --user <user_id> [--session <session_id>] [--json]
  anyclaw pi agents [--user <user_id>] [--json]
  anyclaw pi status
`)
}
