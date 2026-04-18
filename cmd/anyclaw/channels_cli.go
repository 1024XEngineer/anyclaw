package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

func runChannelsCommand(args []string) error {
	if len(args) == 0 {
		return runChannelsList(nil)
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		return runChannelsList(args[1:])
	case "status":
		return runChannelsStatus(args[1:])
	default:
		printChannelsUsage()
		return fmt.Errorf("unknown channels command: %s", args[0])
	}
}

func runChannelsList(args []string) error {
	fs := flag.NewFlagSet("channels list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, items, reachable, err := collectChannelStatuses(*configPath, false)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"gateway_reachable": reachable,
			"count":             len(items),
			"channels":          items,
		})
	}

	if !reachable {
		printInfo("Gateway not reachable at %s; showing configured channels only", gatewayHTTPBaseURL(cfg))
	}
	printSuccess("Found %d channel(s)", len(items))
	printChannelStatuses(items)
	return nil
}

func runChannelsStatus(args []string) error {
	fs := flag.NewFlagSet("channels status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Bool("probe", false, "accepted for OpenClaw CLI compatibility")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, items, reachable, err := collectChannelStatuses(*configPath, true)
	if err != nil {
		return err
	}
	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"gateway_reachable": reachable,
			"count":             len(items),
			"channels":          items,
		})
	}

	printSuccess("Gateway channel status")
	printChannelStatuses(items)
	return nil
}

func printChannelsUsage() {
	fmt.Print(`AnyClaw channels commands:

Usage:
  anyclaw channels list [--json]
  anyclaw channels status [--json]
`)
}

func collectChannelStatuses(configPath string, requireGateway bool) (*config.Config, []inputlayer.Status, bool, error) {
	cfg, err := loadGatewayConfig(configPath)
	if err != nil {
		return nil, nil, false, err
	}

	local := configuredChannelStatuses(cfg)
	ctx, cancel := newGatewayRequestContext()
	defer cancel()

	var remote []inputlayer.Status
	err = doGatewayJSONRequest(ctx, cfg, httpMethodGet, "/channels", nil, &remote)
	if err != nil {
		if requireGateway {
			return nil, nil, false, err
		}
		return cfg, local, false, nil
	}
	return cfg, mergeChannelStatuses(local, remote), true, nil
}

func configuredChannelStatuses(cfg *config.Config) []inputlayer.Status {
	items := map[string]inputlayer.Status{
		"discord":  {Name: "discord", Enabled: cfg.Channels.Discord.Enabled},
		"signal":   {Name: "signal", Enabled: cfg.Channels.Signal.Enabled},
		"slack":    {Name: "slack", Enabled: cfg.Channels.Slack.Enabled},
		"telegram": {Name: "telegram", Enabled: cfg.Channels.Telegram.Enabled},
		"whatsapp": {Name: "whatsapp", Enabled: cfg.Channels.WhatsApp.Enabled},
	}

	registry, err := plugin.NewRegistry(cfg.Plugins)
	if err == nil {
		for _, manifest := range registry.List() {
			if manifest.Builtin || manifest.Channel == nil {
				continue
			}
			name := strings.TrimSpace(manifest.Channel.Name)
			if name == "" {
				name = strings.TrimSpace(manifest.Name)
			}
			if name == "" {
				continue
			}
			lower := strings.ToLower(name)
			if _, exists := items[lower]; exists {
				continue
			}
			items[lower] = inputlayer.Status{Name: name, Enabled: manifest.Enabled}
		}
	}

	merged := make([]inputlayer.Status, 0, len(items))
	for _, item := range items {
		merged = append(merged, item)
	}
	sort.Slice(merged, func(i, j int) bool {
		return strings.ToLower(merged[i].Name) < strings.ToLower(merged[j].Name)
	})
	return merged
}

func mergeChannelStatuses(local []inputlayer.Status, remote []inputlayer.Status) []inputlayer.Status {
	items := map[string]inputlayer.Status{}
	for _, item := range local {
		items[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}
	for _, item := range remote {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		if existing, ok := items[key]; ok {
			if !item.Enabled {
				item.Enabled = existing.Enabled
			}
		}
		items[key] = item
	}

	merged := make([]inputlayer.Status, 0, len(items))
	for _, item := range items {
		merged = append(merged, item)
	}
	sort.Slice(merged, func(i, j int) bool {
		return strings.ToLower(merged[i].Name) < strings.ToLower(merged[j].Name)
	})
	return merged
}

func printChannelStatuses(items []inputlayer.Status) {
	printChannelStatusLines(items, channelStatusPrintOptions{
		DisabledLabel:       "disabled",
		IncludeLastActivity: true,
		LastActivityLabel:   "last_activity=",
		ErrorLabel:          "error=",
	})
}
