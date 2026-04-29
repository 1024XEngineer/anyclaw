package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	gatewaysdk "github.com/1024XEngineer/anyclaw/pkg/gateway/transport/client"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func runPairingCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printPairingUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "generate":
		return runPairingGenerate(ctx, args[1:])
	case "list":
		return runPairingList(ctx, args[1:])
	case "status":
		return runPairingStatus(ctx, args[1:])
	case "renew":
		return runPairingRenew(ctx, args[1:])
	case "unpair":
		return runPairingUnpair(ctx, args[1:])
	default:
		printPairingUsage()
		return fmt.Errorf("unknown pairing command: %s", args[0])
	}
}

func printPairingUsage() {
	fmt.Print(`AnyClaw pairing commands:

Usage:
  anyclaw pairing generate [--config anyclaw.json] [--name <device>] [--type <kind>]
  anyclaw pairing list [--config anyclaw.json]
  anyclaw pairing status [--config anyclaw.json]
  anyclaw pairing renew --device <device_id> [--config anyclaw.json]
  anyclaw pairing unpair --device <device_id> [--config anyclaw.json]
`)
}

func runPairingGenerate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing generate", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	deviceName := fs.String("name", "CLI Device", "device name")
	deviceType := fs.String("type", "cli", "device type")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, cleanup, err := newPairingGatewayClient(ctx, *configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	result, err := client.GeneratePairingCode(ctx, *deviceName, *deviceType)
	if err != nil {
		return err
	}

	fmt.Println(ui.Bold.Sprint("Pairing Code Generated"))
	fmt.Printf("  Code:    %s\n", ui.Cyan.Sprint(result.Code))
	fmt.Printf("  Device:  %s\n", result.Device)
	fmt.Printf("  Type:    %s\n", result.Type)
	fmt.Printf("  Expires: %s\n", result.Expires)
	return nil
}

func runPairingList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, cleanup, err := newPairingGatewayClient(ctx, *configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	devices, err := client.ListPairedDevices(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		printInfo("No paired devices")
		return nil
	}

	fmt.Println(ui.Bold.Sprint("Paired Devices"))
	for i, device := range devices {
		fmt.Printf("  %d. %s\n", i+1, firstString(device, "device_name", "name", "device_id"))
		printMapValue("     ID:     ", device, "device_id")
		printMapValue("     Type:   ", device, "device_type")
		printMapValue("     Status: ", device, "status")
	}
	return nil
}

func runPairingStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, cleanup, err := newPairingGatewayClient(ctx, *configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	status, err := client.GetPairingStatus(ctx)
	if err != nil {
		return err
	}

	fmt.Println(ui.Bold.Sprint("Device Pairing Status"))
	printMapValue("  Enabled:     ", status, "enabled")
	printMapValue("  Max Devices: ", status, "max_devices")
	printMapValue("  Paired:      ", status, "paired")
	printMapValue("  Active:      ", status, "active")
	printMapValue("  Expired:     ", status, "expired")
	printMapValue("  Codes:       ", status, "codes")
	return nil
}

func runPairingRenew(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing renew", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	deviceID := fs.String("device", "", "device ID to renew")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*deviceID) == "" {
		return fmt.Errorf("device ID is required")
	}

	client, cleanup, err := newPairingGatewayClient(ctx, *configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	if _, err := client.RenewPairing(ctx, strings.TrimSpace(*deviceID)); err != nil {
		return err
	}
	printSuccess("Renewed pairing: %s", strings.TrimSpace(*deviceID))
	return nil
}

func runPairingUnpair(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing unpair", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	deviceID := fs.String("device", "", "device ID to unpair")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*deviceID) == "" {
		return fmt.Errorf("device ID is required")
	}

	client, cleanup, err := newPairingGatewayClient(ctx, *configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := client.UnpairDevice(ctx, strings.TrimSpace(*deviceID)); err != nil {
		return err
	}
	printSuccess("Unpaired device: %s", strings.TrimSpace(*deviceID))
	return nil
}

func newPairingGatewayClient(ctx context.Context, configPath string) (*gatewaysdk.WSClient, func(), error) {
	cfg, err := loadGatewayConfig(configPath)
	if err != nil {
		return nil, nil, err
	}

	client := gatewaysdk.NewWSClient(appRuntime.GatewayURL(cfg), cfg.Security.APIToken)
	connectCtx, cancel := context.WithTimeout(ctx, gatewayRequestTimeout)
	if err := client.Connect(connectCtx); err != nil {
		cancel()
		return nil, nil, err
	}

	cleanup := func() {
		cancel()
		_ = client.Close()
	}
	return client, cleanup, nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return "-"
}

func printMapValue(prefix string, values map[string]any, key string) {
	if value, ok := values[key]; ok {
		fmt.Printf("%s%v\n", prefix, value)
	}
}
