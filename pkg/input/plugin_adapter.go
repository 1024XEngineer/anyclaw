package input

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

type pluginChannelAdapter struct {
	base        BaseAdapter
	runner      plugin.ChannelRunner
	appendEvent func(eventType string, sessionID string, payload map[string]any)
}

// NewPluginChannelAdapter adapts a plugin-backed channel runner into the input layer contract.
func NewPluginChannelAdapter(runner plugin.ChannelRunner, appendEvent func(eventType string, sessionID string, payload map[string]any)) Adapter {
	return &pluginChannelAdapter{
		base:        NewBaseAdapter(runner.Manifest.Name, true),
		runner:      runner,
		appendEvent: appendEvent,
	}
}

func (a *pluginChannelAdapter) Name() string  { return a.runner.Manifest.Name }
func (a *pluginChannelAdapter) Enabled() bool { return true }

func (a *pluginChannelAdapter) Status() Status {
	status := a.base.Status()
	status.Enabled = true
	return status
}

func (a *pluginChannelAdapter) Run(ctx context.Context, handle InboundHandler) error {
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

func (a *pluginChannelAdapter) pollOnce(ctx context.Context, handle InboundHandler) error {
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
		sessionID, _, err := handle(ctx, "", item.Message, map[string]string{
			"channel":      a.runner.Manifest.Channel.Name,
			"source":       item.Source,
			"reply_target": item.Source,
		})
		if err != nil {
			return err
		}
		a.base.MarkActivity()
		a.append("channel.plugin.message", sessionID, map[string]any{
			"plugin":  a.runner.Manifest.Name,
			"channel": a.runner.Manifest.Channel.Name,
			"source":  item.Source,
			"message": item.Message,
		})
	}

	return nil
}

func (a *pluginChannelAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}
