package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/setup"
)

func TestRunAnyClawCLIRoutesDoctorCommand(t *testing.T) {
	clearModelsCLIEnv(t)

	originalRunDoctor := runDoctorSetup
	t.Cleanup(func() {
		runDoctorSetup = originalRunDoctor
	})

	called := false
	runDoctorSetup = func(ctx context.Context, configPath string, opts setup.DoctorOptions) (*setup.Report, *config.Config, error) {
		called = true
		if configPath != "anyclaw.json" {
			t.Fatalf("expected default config path, got %q", configPath)
		}
		if !opts.CheckConnectivity {
			t.Fatal("expected connectivity check enabled by default")
		}
		report := &setup.Report{}
		report.Add(setup.CheckResult{
			ID:       "missing-key",
			Title:    "Missing API key",
			Severity: setup.SeverityWarning,
			Message:  "Set an API key before chatting",
		})
		return report, config.DefaultConfig(), nil
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"doctor"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI doctor: %v", err)
	}
	if !called {
		t.Fatal("expected doctor command to invoke setup doctor runner")
	}
	for _, want := range []string{
		"AnyClaw doctor",
		"Missing API key",
		"Summary: 0 error(s), 1 warning(s)",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunAnyClawCLIRoutesOnboardAndSetupAlias(t *testing.T) {
	clearModelsCLIEnv(t)

	configPath := filepath.Join(t.TempDir(), "custom.json")
	originalRunOnboarding := runOnboardingSetup
	originalDetectInteractive := detectTerminalInteractive
	t.Cleanup(func() {
		runOnboardingSetup = originalRunOnboarding
		detectTerminalInteractive = originalDetectInteractive
	})

	calls := 0
	runOnboardingSetup = func(ctx context.Context, gotConfigPath string, opts setup.OnboardOptions) (*setup.OnboardResult, error) {
		calls++
		if gotConfigPath != configPath {
			t.Fatalf("expected config path %q, got %q", configPath, gotConfigPath)
		}
		if opts.Interactive {
			t.Fatal("expected --non-interactive to disable prompts")
		}
		return &setup.OnboardResult{
			Config: config.DefaultConfig(),
		}, nil
	}
	detectTerminalInteractive = func() bool { return true }

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"onboard", "--config", configPath, "--non-interactive"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI onboard: %v", err)
	}
	if !strings.Contains(stdout, "Onboarding wrote: "+config.ResolveConfigPath(configPath)) {
		t.Fatalf("expected onboarding output, got %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"setup", "--config", configPath, "--non-interactive"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI setup alias: %v", err)
	}
	if !strings.Contains(stdout, "Onboarding wrote: "+config.ResolveConfigPath(configPath)) {
		t.Fatalf("expected setup alias output, got %q", stdout)
	}
	if calls != 2 {
		t.Fatalf("expected onboarding runner to be called twice, got %d", calls)
	}
}

func TestRunAnyClawCLIHelpDocumentsSetupAlias(t *testing.T) {
	clearModelsCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI help: %v", err)
	}
	if !strings.Contains(stdout, "anyclaw onboard/setup [options]") {
		t.Fatalf("expected setup alias in help output, got %q", stdout)
	}
}

func TestPrintDoctorReportIncludesErrorWarningDetailAndHint(t *testing.T) {
	report := &setup.Report{}
	report.Add(setup.CheckResult{
		ID:       "error-check",
		Title:    "Config missing",
		Severity: setup.SeverityError,
		Message:  "Set up a provider",
		Detail:   "No default provider configured",
		Hint:     "Run anyclaw onboard",
	})
	report.Add(setup.CheckResult{
		ID:       "warn-check",
		Title:    "Connectivity skipped",
		Severity: setup.SeverityWarning,
		Message:  "No network check performed",
	})

	stdout, _, err := captureCLIOutput(t, func() error {
		printDoctorReport(report)
		return nil
	})
	if err != nil {
		t.Fatalf("printDoctorReport: %v", err)
	}
	for _, want := range []string{
		"Error: Config missing: Set up a provider",
		"Warning:",
		"Connectivity skipped: No network check performed",
		"No default provider configured",
		"hint: Run anyclaw onboard",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestEnsureConfigOnboardedWarnsWhenExistingConfigNeedsSetupAndNonInteractive(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	originalRunOnboarding := runOnboardingSetup
	originalDetectInteractive := detectTerminalInteractive
	t.Cleanup(func() {
		runOnboardingSetup = originalRunOnboarding
		detectTerminalInteractive = originalDetectInteractive
	})

	detectTerminalInteractive = func() bool { return false }
	runOnboardingSetup = func(ctx context.Context, gotConfigPath string, opts setup.OnboardOptions) (*setup.OnboardResult, error) {
		t.Fatal("did not expect onboarding runner to be called")
		return nil, nil
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return ensureConfigOnboarded(context.Background(), configPath, true)
	})
	if err != nil {
		t.Fatalf("ensureConfigOnboarded: %v", err)
	}
	if !strings.Contains(stdout, "Config exists but model setup is incomplete.") {
		t.Fatalf("expected warning output, got %q", stdout)
	}
}

func TestEnsureConfigOnboardedRunsOnboardingForMissingConfig(t *testing.T) {
	clearModelsCLIEnv(t)

	configPath := filepath.Join(t.TempDir(), "missing.json")
	originalRunOnboarding := runOnboardingSetup
	originalDetectInteractive := detectTerminalInteractive
	t.Cleanup(func() {
		runOnboardingSetup = originalRunOnboarding
		detectTerminalInteractive = originalDetectInteractive
	})

	called := false
	detectTerminalInteractive = func() bool { return false }
	runOnboardingSetup = func(ctx context.Context, gotConfigPath string, opts setup.OnboardOptions) (*setup.OnboardResult, error) {
		called = true
		if gotConfigPath != configPath {
			t.Fatalf("expected config path %q, got %q", configPath, gotConfigPath)
		}
		if opts.Interactive {
			t.Fatal("expected missing-config onboarding to be non-interactive in test")
		}
		return &setup.OnboardResult{Config: config.DefaultConfig()}, nil
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return ensureConfigOnboarded(context.Background(), configPath, true)
	})
	if err != nil {
		t.Fatalf("ensureConfigOnboarded missing config: %v", err)
	}
	if !called {
		t.Fatal("expected onboarding runner to be called for missing config")
	}
	if !strings.Contains(stdout, "No config found. Running first-run onboarding.") {
		t.Fatalf("expected onboarding notice, got %q", stdout)
	}
}

func TestConfigNeedsProviderSetupHonorsProviderProfile(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.Providers = []config.ProviderProfile{
		{
			ID:       "openai-main",
			Name:     "OpenAI Main",
			Provider: "openai",
			APIKey:   "secret-key",
			Enabled:  config.BoolPtr(true),
		},
	}
	cfg.LLM.DefaultProviderRef = "openai-main"
	cfg.LLM.Provider = "openai"
	cfg.LLM.APIKey = ""

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	needsSetup, err := configNeedsProviderSetup(configPath)
	if err != nil {
		t.Fatalf("configNeedsProviderSetup: %v", err)
	}
	if needsSetup {
		t.Fatal("expected provider profile with API key to satisfy setup requirements")
	}
}

func TestConfigNeedsProviderSetupRequiresCompatibleBaseURL(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.LLM.Provider = "compatible"
	cfg.LLM.BaseURL = ""

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	needsSetup, err := configNeedsProviderSetup(configPath)
	if err != nil {
		t.Fatalf("configNeedsProviderSetup: %v", err)
	}
	if !needsSetup {
		t.Fatal("expected compatible provider without base URL to require setup")
	}
}

func TestTerminalInteractiveHandlesClosedStdin(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close temp stdin: %v", err)
	}

	origStdin := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = origStdin
	})

	if terminalInteractive() {
		t.Fatal("expected closed file stdin to be treated as non-interactive")
	}
}
