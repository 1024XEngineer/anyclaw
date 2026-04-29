package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

func withStoreCLITempDir(t *testing.T) {
	t.Helper()
	clearModelsCLIEnv(t)
	t.Setenv("ANYCLAW_SKILLS_DIR", "")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
}

func TestRunAnyClawCLIRoutesStoreUsage(t *testing.T) {
	withStoreCLITempDir(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw store commands:") {
		t.Fatalf("expected store usage output, got %q", stdout)
	}
}

func TestRunStoreListPrintsBuiltinPackages(t *testing.T) {
	withStoreCLITempDir(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "list"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store list: %v", err)
	}
	for _, want := range []string{
		"Packages (",
		"go-expert",
		"category:",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunStoreSearchAndInfoUseCatalog(t *testing.T) {
	withStoreCLITempDir(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "search", "go"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store search: %v", err)
	}
	if !strings.Contains(stdout, "Results (") || !strings.Contains(stdout, "anyclaw store install go-expert") {
		t.Fatalf("unexpected search output: %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "info", "go-expert"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store info: %v", err)
	}
	for _, want := range []string{
		"id:          go-expert",
		"system prompt:",
		"install: anyclaw store install go-expert",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunStoreInstallAndUninstallBuiltinSkill(t *testing.T) {
	withStoreCLITempDir(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "install", "go-expert"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store install: %v", err)
	}
	if !strings.Contains(stdout, "Installed:") {
		t.Fatalf("unexpected install output: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join("skills", "go-expert", "skill.json")); err != nil {
		t.Fatalf("expected installed skill.json: %v", err)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "uninstall", "go-expert"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store uninstall: %v", err)
	}
	if !strings.Contains(stdout, "Uninstalled: go-expert") {
		t.Fatalf("unexpected uninstall output: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join("skills", "go-expert")); !os.IsNotExist(err) {
		t.Fatalf("expected installed skill directory to be removed, got %v", err)
	}
}

func TestRunStoreSourcesAddAndList(t *testing.T) {
	withStoreCLITempDir(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "sources", "add", "local", "https://example.test/plugins.json"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store sources add: %v", err)
	}
	if !strings.Contains(stdout, "Added source: local -> https://example.test/plugins.json") {
		t.Fatalf("unexpected sources add output: %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "sources"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store sources: %v", err)
	}
	if !strings.Contains(stdout, "local: https://example.test/plugins.json (http)") {
		t.Fatalf("unexpected sources output: %q", stdout)
	}
}

func TestRunStoreSignVerifyAndTrustPlugin(t *testing.T) {
	withStoreCLITempDir(t)

	keyPair, err := plugin.GenerateKeyPair(plugin.SignerTypeRSA, 2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	keyData, err := json.Marshal(keyPair)
	if err != nil {
		t.Fatalf("Marshal keyPair: %v", err)
	}
	if err := os.WriteFile("key.json", keyData, 0o600); err != nil {
		t.Fatalf("WriteFile key.json: %v", err)
	}
	if err := os.WriteFile("public.pem", []byte(keyPair.PublicKey), 0o600); err != nil {
		t.Fatalf("WriteFile public.pem: %v", err)
	}

	pluginDir := "demo-plugin"
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll pluginDir: %v", err)
	}
	manifest := plugin.Manifest{
		Name:        "demo-plugin",
		Version:     "1.0.0",
		Description: "Demo plugin",
		Kinds:       []string{"tool"},
		Signer:      "dev-local",
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), manifestData, 0o644); err != nil {
		t.Fatalf("WriteFile plugin manifest: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "sign", pluginDir, "key.json"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store sign: %v", err)
	}
	if !strings.Contains(stdout, "Plugin signed successfully") {
		t.Fatalf("unexpected sign output: %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "verify", pluginDir, "public.pem"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store verify: %v", err)
	}
	if !strings.Contains(stdout, "Plugin signature is VALID") || !strings.Contains(stdout, "Key ID: "+keyPair.KeyID) {
		t.Fatalf("unexpected verify output: %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"store", "trust", keyPair.KeyID, "public.pem", "Dev Local"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI store trust: %v", err)
	}
	if !strings.Contains(stdout, "Added "+keyPair.KeyID+" to trusted signers") {
		t.Fatalf("unexpected trust output: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(".anyclaw", "trust.json")); err != nil {
		t.Fatalf("expected trust store to be written: %v", err)
	}
}
