package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/setup"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
)

var runDoctorSetup = setup.RunDoctor
var runOnboardingSetup = setup.RunOnboarding
var providerNeedsSetupAPIKey = setup.ProviderNeedsAPIKey
var detectTerminalInteractive = terminalInteractive

func terminalInteractive() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	stdoutInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stdinInfo.Mode()&os.ModeCharDevice) != 0 && (stdoutInfo.Mode()&os.ModeCharDevice) != 0
}

func runDoctorCommand(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	repair := fs.Bool("repair", false, "create missing directories while checking")
	connectivity := fs.Bool("connectivity", true, "run a live model connectivity check")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Printf("%s\n", ui.Bold.Sprint("AnyClaw doctor"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	report, _, err := runDoctorSetup(context.Background(), *configPath, setup.DoctorOptions{
		CheckConnectivity: *connectivity,
		CreateMissingDirs: *repair,
	})
	printDoctorReport(report)
	if report != nil {
		printInfo("Summary: %d error(s), %d warning(s)", report.ErrorCount(), report.WarningCount())
	}
	return err
}

func runOnboardCommand(args []string) error {
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	nonInteractive := fs.Bool("non-interactive", false, "write defaults without prompting")
	connectivity := fs.Bool("connectivity", true, "run a live model connectivity check after saving")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := runOnboardingSetup(context.Background(), *configPath, setup.OnboardOptions{
		Interactive:       !*nonInteractive && detectTerminalInteractive(),
		CheckConnectivity: *connectivity,
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
	})
	if result != nil {
		printDoctorReport(result.Report)
		printSuccess("Onboarding wrote: %s", config.ResolveConfigPath(*configPath))
	}
	return err
}

func printDoctorReport(report *setup.Report) {
	if report == nil {
		return
	}
	for _, check := range report.Checks {
		switch check.Severity {
		case setup.SeverityError:
			printError("%s: %s", check.Title, check.Message)
		case setup.SeverityWarning:
			fmt.Printf("%s\n", ui.Warning.Sprint("! Warning: ")+check.Title+": "+check.Message)
		default:
			printSuccess("%s: %s", check.Title, check.Message)
		}
		if check.Detail != "" {
			fmt.Printf("    %s\n", ui.Dim.Sprint(check.Detail))
		}
		if check.Hint != "" {
			fmt.Printf("    hint: %s\n", check.Hint)
		}
	}
}

func ensureConfigOnboarded(ctx context.Context, configPath string, checkConnectivity bool) error {
	if _, err := os.Stat(configPath); err == nil {
		needsSetup, setupErr := configNeedsProviderSetup(configPath)
		if setupErr != nil {
			return setupErr
		}
		if !needsSetup {
			return nil
		}

		if !detectTerminalInteractive() {
			printWarn("Config exists but model setup is incomplete. Run `anyclaw onboard` or fill your provider Base URL / API key before chatting.")
			return nil
		}

		printInfo("First-run model setup required. Please choose a provider and enter Base URL / API key.")
		result, err := runOnboardingSetup(ctx, configPath, setup.OnboardOptions{
			Interactive:       true,
			CheckConnectivity: checkConnectivity,
			Stdin:             os.Stdin,
			Stdout:            os.Stdout,
		})
		if result != nil {
			printDoctorReport(result.Report)
		}
		return err
	} else if !os.IsNotExist(err) {
		return err
	}

	printInfo("No config found. Running first-run onboarding.")
	result, err := runOnboardingSetup(ctx, configPath, setup.OnboardOptions{
		Interactive:       detectTerminalInteractive(),
		CheckConnectivity: checkConnectivity,
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
	})
	if result != nil {
		printDoctorReport(result.Report)
	}
	return err
}

func configNeedsProviderSetup(configPath string) (bool, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return false, err
	}

	provider := strings.TrimSpace(cfg.LLM.Provider)
	apiKey := strings.TrimSpace(cfg.LLM.APIKey)
	baseURL := strings.TrimSpace(cfg.LLM.BaseURL)
	if profile, ok := cfg.FindDefaultProviderProfile(); ok {
		if value := strings.TrimSpace(profile.Provider); value != "" {
			provider = value
		}
		if value := strings.TrimSpace(profile.APIKey); value != "" {
			apiKey = value
		}
		if value := strings.TrimSpace(profile.BaseURL); value != "" {
			baseURL = value
		}
	}

	if provider == "" {
		return true, nil
	}
	if strings.EqualFold(provider, "compatible") && baseURL == "" {
		return true, nil
	}
	if providerNeedsSetupAPIKey(provider) && apiKey == "" {
		return true, nil
	}
	return false, nil
}
