package main

import (
	"context"
	"fmt"
	"os"

	"github.com/anyclaw/anyclaw/pkg/setup"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

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
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	printInfo("No config found. Running first-run onboarding.")
	result, err := setup.RunOnboarding(ctx, configPath, setup.OnboardOptions{
		Interactive:       terminalInteractive(),
		CheckConnectivity: checkConnectivity,
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
	})
	if result != nil {
		printDoctorReport(result.Report)
	}
	return err
}
