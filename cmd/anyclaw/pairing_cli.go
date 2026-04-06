package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/gateway"
	"github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runPairingCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printPairingUsage()
		return nil
	}

	switch normalizePairingCommand(args[0]) {
	case "generate":
		return runPairingGenerate(ctx, args[1:])
	case "list":
		return runPairingList(ctx, args[1:])
	case "status":
		return runPairingStatus(ctx, args[1:])
	case "unpair":
		return runPairingUnpair(ctx, args[1:])
	case "renew":
		return runPairingRenew(ctx, args[1:])
	default:
		printPairingUsage()
		return fmt.Errorf("unknown pairing command: %s", args[0])
	}
}

func normalizePairingCommand(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func runPairingGenerate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing generate", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	deviceName := fs.String("name", "CLI Device", "device name")
	deviceType := fs.String("type", "cli", "device type (cli, mobile, desktop)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	gatewayURL := runtime.GatewayURL(cfg)
	client := gateway.NewWSClient(gatewayURL, cfg.Security.APIToken)
	if client == nil {
		return fmt.Errorf("failed to create Gateway client")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Gateway: %w", err)
	}
	defer client.Close()

	result, err := client.GeneratePairingCode(ctx, *deviceName, *deviceType)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("Pairing Code Generated"))
	fmt.Printf("  Code:     %s\n", ui.Cyan.Sprint(result.Code))
	fmt.Printf("  Device:   %s\n", result.Device)
	fmt.Printf("  Type:     %s\n", result.Type)
	fmt.Printf("  Expires:  %s\n", result.Expires)
	fmt.Println()
	fmt.Printf("%s\n", ui.Info.Sprint("Share this code with your device within the TTL period."))
	return nil
}

func runPairingList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	gatewayURL := runtime.GatewayURL(cfg)
	client := gateway.NewWSClient(gatewayURL, cfg.Security.APIToken)
	if client == nil {
		return fmt.Errorf("failed to create Gateway client")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Gateway: %w", err)
	}
	defer client.Close()

	devices, err := client.ListPairedDevices(ctx)
	if err != nil {
		return err
	}

	fmt.Println()
	if len(devices) == 0 {
		fmt.Printf("%s\n", ui.Info.Sprint("No paired devices"))
		return nil
	}

	fmt.Printf("%s\n", ui.Bold.Sprint("Paired Devices"))
	for i, device := range devices {
		fmt.Printf("\n  %d. %s\n", i+1, ui.Cyan.Sprint(device["device_name"]))
		if id, ok := device["device_id"].(string); ok {
			fmt.Printf("     ID:   %s\n", id)
		}
		if dt, ok := device["device_type"].(string); ok {
			fmt.Printf("     Type: %s\n", dt)
		}
		if paired, ok := device["paired_at"].(string); ok {
			fmt.Printf("     Paired: %s\n", paired)
		}
		if status, ok := device["status"].(string); ok {
			fmt.Printf("     Status: %s\n", status)
		}
	}
	fmt.Println()
	return nil
}

func runPairingStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pairing status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	gatewayURL := runtime.GatewayURL(cfg)
	client := gateway.NewWSClient(gatewayURL, cfg.Security.APIToken)
	if client == nil {
		return fmt.Errorf("failed to create Gateway client")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Gateway: %w", err)
	}
	defer client.Close()

	status, err := client.GetPairingStatus(ctx)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("Device Pairing Status"))
	fmt.Printf("  Enabled:     %v\n", status["enabled"])
	fmt.Printf("  Max Devices: %v\n", status["max_devices"])
	fmt.Printf("  Paired:      %v\n", status["paired"])
	fmt.Printf("  Active:      %v\n", status["active"])
	fmt.Printf("  Expired:     %v\n", status["expired"])
	fmt.Printf("  Codes:       %v\n", status["codes"])
	fmt.Println()
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

	if *deviceID == "" {
		return fmt.Errorf("device ID is required (use --device flag)")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	gatewayURL := runtime.GatewayURL(cfg)
	client := gateway.NewWSClient(gatewayURL, cfg.Security.APIToken)
	if client == nil {
		return fmt.Errorf("failed to create Gateway client")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Gateway: %w", err)
	}
	defer client.Close()

	if err := client.UnpairDevice(ctx, *deviceID); err != nil {
		return err
	}

	printSuccess("Device unpaired: %s", *deviceID)
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

	if *deviceID == "" {
		return fmt.Errorf("device ID is required (use --device flag)")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	gatewayURL := runtime.GatewayURL(cfg)
	client := gateway.NewWSClient(gatewayURL, cfg.Security.APIToken)
	if client == nil {
		return fmt.Errorf("failed to create Gateway client")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to Gateway: %w", err)
	}
	defer client.Close()

	result, err := client.RenewPairing(ctx, *deviceID)
	if err != nil {
		return err
	}

	printSuccess("Pairing renewed for device: %s", *deviceID)
	if expires, ok := result["expires_at"].(string); ok {
		fmt.Printf("  New expiry: %s\n", expires)
	}
	return nil
}

func printPairingUsage() {
	fmt.Print(`AnyClaw device pairing commands:

Usage:
  anyclaw pairing generate [--name "Device Name"] [--type cli|mobile|desktop]
  anyclaw pairing list
  anyclaw pairing status
  anyclaw pairing unpair --device <device_id>
  anyclaw pairing renew --device <device_id>

Examples:
  anyclaw pairing generate --name "My Laptop" --type desktop
  anyclaw pairing list
  anyclaw pairing status
  anyclaw pairing unpair --device node-123456789
  anyclaw pairing renew --device node-123456789
`)
}
