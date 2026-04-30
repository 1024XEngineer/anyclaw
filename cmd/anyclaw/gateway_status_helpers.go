package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
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

func isGatewayReachabilityError(err error) bool {
	if err == nil {
		return false
	}
	var urlErr *url.Error
	var netErr net.Error
	if !errors.As(err, &urlErr) && !errors.As(err, &netErr) {
		return false
	}
	return errorsContainAny(err.Error(),
		"connection refused",
		"actively refused",
		"no such host",
		"server misbehaving",
		"timeout",
		"deadline exceeded",
		"connectex",
	)
}

func errorsContainAny(message string, fragments ...string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	for _, fragment := range fragments {
		if strings.Contains(lower, strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func printGatewayUnreachableHint(cfg *config.Config, commandName string) {
	printInfo("Gateway not reachable at %s", gatewayHTTPBaseURL(cfg))
	printInfo("Start it with `anyclaw gateway start` or `anyclaw gateway daemon start`, then rerun `anyclaw %s`", commandName)
}

func channelStateLabel(item inputlayer.Status, disabledLabel string) string {
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

func printChannelStatusLines(items []inputlayer.Status, opts channelStatusPrintOptions) {
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
