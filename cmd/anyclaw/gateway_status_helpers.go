package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/config"
)

const gatewayRequestTimeout = 5 * time.Second

type channelStatusPrintOptions struct {
	DisabledLabel       string
	IncludeLastActivity bool
	LastActivityLabel   string
	ErrorLabel          string
}

func loadGatewayConfig(configPath string) (*config.Config, error) {
	return config.Load(configPath)
}

func newGatewayRequestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), gatewayRequestTimeout)
}

func channelStateLabel(item channel.Status, disabledLabel string) string {
	switch {
	case item.Enabled && item.Running && item.Healthy:
		return "healthy"
	case item.Enabled && item.Running:
		return "running"
	case item.Enabled:
		return "enabled"
	default:
		return disabledLabel
	}
}

func printChannelStatusLines(items []channel.Status, opts channelStatusPrintOptions) {
	for _, item := range items {
		fmt.Printf("  - %s: %s\n", item.Name, channelStateLabel(item, opts.DisabledLabel))
		if opts.IncludeLastActivity && !item.LastActivity.IsZero() {
			fmt.Printf("    %s%s\n", opts.LastActivityLabel, item.LastActivity.Format(time.RFC3339))
		}
		if strings.TrimSpace(item.LastError) != "" {
			fmt.Printf("    %s%s\n", opts.ErrorLabel, item.LastError)
		}
	}
}
