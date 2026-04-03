package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/plugin"
)

type pluginChannelAdapter struct {
	base        channel.BaseAdapter
	runner      plugin.ChannelRunner
	appendEvent func(eventType string, sessionID string, payload map[string]any)
	router      *channel.Router
	sessions    map[string]string
}

func newPluginChannelAdapter(runner plugin.ChannelRunner, router *channel.Router, appendEvent func(eventType string, sessionID string, payload map[string]any)) channel.Adapter {
	return &pluginChannelAdapter{
		base:        channel.NewBaseAdapter(runner.Manifest.Name, true),
		runner:      runner,
		appendEvent: appendEvent,
		router:      router,
		sessions:    make(map[string]string),
	}
}

func (a *pluginChannelAdapter) Name() string  { return a.runner.Manifest.Name }
func (a *pluginChannelAdapter) Enabled() bool { return true }
func (a *pluginChannelAdapter) Status() channel.Status {
	status := a.base.Status()
	status.Enabled = true
	return status
}

func (a *pluginChannelAdapter) Run(ctx context.Context, handle channel.InboundHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)
	ticker := time.NewTicker(a.runner.Timeout)
	defer ticker.Stop()
	for {
		if err := a.pollOnce(ctx, handle); err != nil {
			a.base.SetError(err)
			a.append("channel.plugin.error", "", map[string]any{"plugin": a.runner.Manifest.Name, "error": err.Error()})
		} else {
			a.base.SetError(nil)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (a *pluginChannelAdapter) pollOnce(ctx context.Context, handle channel.InboundHandler) error {
	ctx, cancel := context.WithTimeout(ctx, a.runner.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.runner.Entrypoint)
	pluginDir := filepath.Dir(a.runner.Entrypoint)
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(),
		"ANYCLAW_PLUGIN_MODE=channel-poll",
		"ANYCLAW_PLUGIN_DIR="+pluginDir,
		"ANYCLAW_PLUGIN_TIMEOUT_SECONDS="+fmt.Sprintf("%d", int(a.runner.Timeout/time.Second)),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("channel plugin timed out after %s", a.runner.Timeout)
		}
		return fmt.Errorf("channel plugin failed: %w: %s", err, string(output))
	}
	var messages []struct {
		Source  string `json:"source"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(output, &messages); err != nil {
		return err
	}
	for _, item := range messages {
		decision := a.router.Decide(channel.RouteRequest{Channel: a.runner.Manifest.Channel.Name, Source: item.Source, Text: item.Message})
		sessionID := a.sessions[decision.Key]
		if decision.SessionID != "" {
			sessionID = decision.SessionID
		}
		sessionID, _, err := handle(ctx, sessionID, item.Message, map[string]string{"channel": a.runner.Manifest.Channel.Name, "source": item.Source, "reply_target": item.Source})
		if err != nil {
			return err
		}
		if sessionID != "" {
			a.sessions[decision.Key] = sessionID
		}
		a.base.MarkActivity()
		a.append("channel.plugin.message", sessionID, map[string]any{
			"plugin":    a.runner.Manifest.Name,
			"channel":   a.runner.Manifest.Channel.Name,
			"source":    item.Source,
			"message":   item.Message,
			"route":     decision.Key,
			"agent":     decision.Agent,
			"workspace": decision.Workspace,
		})
	}
	return nil
}

func (a *pluginChannelAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}
