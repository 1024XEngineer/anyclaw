package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentstore "github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
)

func runStoreCommand(args []string) error {
	if len(args) == 0 {
		printStoreUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		return runStoreList(args[1:])
	case "search":
		return runStoreSearch(args[1:])
	case "info":
		return runStoreInfo(args[1:])
	case "install":
		return runStoreInstall(args[1:])
	case "uninstall":
		return runStoreUninstall(args[1:])
	case "sign":
		return runStoreSign(args[1:])
	case "verify":
		return runStoreVerify(args[1:])
	case "trust":
		return runStoreTrust(args[1:])
	case "sources":
		return runStoreSources(args[1:])
	case "update":
		return runStoreUpdate(args[1:])
	default:
		printStoreUsage()
		return fmt.Errorf("unknown store command: %s", args[0])
	}
}

func printStoreUsage() {
	fmt.Print(`AnyClaw store commands:

Usage:
  anyclaw store list [category]
  anyclaw store search <keyword>
  anyclaw store info <id>
  anyclaw store install <id>
  anyclaw store uninstall <id>
  anyclaw store sign <plugin-dir> <key-file>
  anyclaw store verify <plugin-dir> <public-key-file>
  anyclaw store trust <key-id> <public-key-file> [name]
  anyclaw store sources [add <name> <url>]
  anyclaw store update [plugin-id]
`)
}

func newStoreManager() (agentstore.StoreManager, error) {
	return agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
}

func runStoreList(args []string) error {
	sm, err := newStoreManager()
	if err != nil {
		return err
	}

	filter := agentstore.StoreFilter{}
	if len(args) > 0 {
		filter.Category = args[0]
	}

	packages := sm.List(filter)
	if len(packages) == 0 {
		printInfo("No packages found.")
		return nil
	}

	fmt.Println(ui.Bold.Sprint(fmt.Sprintf("Packages (%d):", len(packages))))
	fmt.Println()
	for _, pkg := range packages {
		icon := pkg.Icon
		if icon == "" {
			icon = "-"
		}
		installed := ""
		if sm.IsInstalled(pkg.ID) {
			installed = ui.Green.Sprint(" [installed]")
		}
		fmt.Println("  " + icon + " " + ui.Bold.Sprint(pkg.DisplayName) + installed)
		fmt.Println("     " + ui.Dim.Sprint(pkg.Description))
		fmt.Println(fmt.Sprintf("     category: %s | rating: %.1f (%d) | downloads: %d", pkg.Category, pkg.Rating, pkg.RatingCount, pkg.Downloads))
		fmt.Println("     " + ui.Dim.Sprint(fmt.Sprintf("id: %s", pkg.ID)))
		fmt.Println()
	}
	return nil
}

func runStoreSearch(args []string) error {
	keyword := strings.TrimSpace(strings.Join(args, " "))
	if keyword == "" {
		return fmt.Errorf("usage: anyclaw store search <keyword>")
	}

	sm, err := newStoreManager()
	if err != nil {
		return err
	}

	results := sm.Search(keyword)
	if len(results) == 0 {
		fmt.Printf("No results for %q.\n", keyword)
		return nil
	}

	fmt.Println(ui.Bold.Sprint(fmt.Sprintf("Results (%d):", len(results))))
	fmt.Println()
	for _, pkg := range results {
		icon := pkg.Icon
		if icon == "" {
			icon = "-"
		}
		installed := ""
		if sm.IsInstalled(pkg.ID) {
			installed = ui.Green.Sprint(" [installed]")
		}
		fmt.Println("  " + icon + " " + ui.Bold.Sprint(pkg.DisplayName) + installed)
		fmt.Println("     " + ui.Dim.Sprint(pkg.Description))
		fmt.Println("     " + ui.Dim.Sprint(fmt.Sprintf("install: anyclaw store install %s", pkg.ID)))
		fmt.Println()
	}
	return nil
}

func runStoreInfo(args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: anyclaw store info <id>")
	}

	sm, err := newStoreManager()
	if err != nil {
		return err
	}

	pkg, err := sm.Get(args[0])
	if err != nil {
		return err
	}

	icon := pkg.Icon
	if icon == "" {
		icon = "-"
	}

	fmt.Println(icon + " " + ui.Bold.Sprint(pkg.DisplayName))
	fmt.Println()
	fmt.Println(fmt.Sprintf("  id:          %s", pkg.ID))
	fmt.Println(fmt.Sprintf("  description: %s", pkg.Description))
	fmt.Println(fmt.Sprintf("  author:      %s", pkg.Author))
	fmt.Println(fmt.Sprintf("  version:     %s", pkg.Version))
	fmt.Println(fmt.Sprintf("  category:    %s", pkg.Category))
	fmt.Println(fmt.Sprintf("  tags:        %s", strings.Join(pkg.Tags, ", ")))
	fmt.Println(fmt.Sprintf("  domain:      %s", pkg.Domain))
	fmt.Println(fmt.Sprintf("  expertise:   %s", strings.Join(pkg.Expertise, ", ")))
	fmt.Println(fmt.Sprintf("  skills:      %s", strings.Join(pkg.Skills, ", ")))
	fmt.Println(fmt.Sprintf("  permission:  %s", pkg.Permission))
	fmt.Println(fmt.Sprintf("  rating:      %.1f (%d)", pkg.Rating, pkg.RatingCount))
	fmt.Println(fmt.Sprintf("  downloads:   %d", pkg.Downloads))
	fmt.Println(fmt.Sprintf("  tone:        %s", pkg.Tone))
	fmt.Println(fmt.Sprintf("  style:       %s", pkg.Style))
	fmt.Println()
	fmt.Println("  system prompt:")
	fmt.Println("    " + ui.Dim.Sprint(pkg.SystemPrompt))
	fmt.Println()
	if sm.IsInstalled(pkg.ID) {
		fmt.Println("  " + ui.Green.Sprint("installed"))
	} else {
		fmt.Println("  " + ui.Dim.Sprint(fmt.Sprintf("install: anyclaw store install %s", pkg.ID)))
	}
	return nil
}

func runStoreInstall(args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: anyclaw store install <id>")
	}

	sm, err := newStoreManager()
	if err != nil {
		return err
	}

	id := args[0]
	if sm.IsInstalled(id) {
		printInfo("Already installed: %s", id)
		return nil
	}

	if err := sm.Install(id); err != nil {
		return err
	}

	pkg, _ := sm.Get(id)
	if pkg != nil {
		printSuccess("Installed: %s (%s)", pkg.DisplayName, id)
	} else {
		printSuccess("Installed: %s", id)
	}
	return nil
}

func runStoreUninstall(args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: anyclaw store uninstall <id>")
	}

	sm, err := newStoreManager()
	if err != nil {
		return err
	}

	if err := sm.Uninstall(args[0]); err != nil {
		return err
	}
	printSuccess("Uninstalled: %s", args[0])
	return nil
}

func runStoreSign(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: anyclaw store sign <plugin-dir> <key-file>")
	}

	keyData, err := os.ReadFile(args[1])
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	var keyPair plugin.KeyPair
	if err := json.Unmarshal(keyData, &keyPair); err != nil {
		return fmt.Errorf("failed to parse key file: %w", err)
	}

	sig, err := plugin.SignPluginDir(args[0], &keyPair)
	if err != nil {
		return fmt.Errorf("sign failed: %w", err)
	}

	if err := plugin.SaveSignature(args[0], sig); err != nil {
		return fmt.Errorf("failed to save signature: %w", err)
	}

	printSuccess("Plugin signed successfully!")
	return nil
}

func runStoreVerify(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: anyclaw store verify <plugin-dir> <public-key-file>")
	}

	sig, err := plugin.LoadSignature(args[0])
	if err != nil {
		return fmt.Errorf("failed to load signature: %w", err)
	}

	keyData, err := os.ReadFile(args[1])
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	verified, err := plugin.VerifyPluginDir(args[0], sig, string(keyData))
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	if verified {
		fmt.Println("Plugin signature is VALID!")
		fmt.Printf("  Signer: %s\n", sig.Signer)
		fmt.Printf("  Key ID: %s\n", sig.KeyID)
		fmt.Printf("  Signed: %s\n", sig.Timestamp)
	} else {
		fmt.Println("Plugin signature is INVALID!")
	}
	return nil
}

func runStoreTrust(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: anyclaw store trust <key-id> <public-key-file> [name]")
	}

	keyID := strings.TrimSpace(args[0])
	if keyID == "" {
		return fmt.Errorf("key id is required")
	}
	if _, err := os.ReadFile(args[1]); err != nil {
		return fmt.Errorf("failed to read public key file: %w", err)
	}

	trustStore := plugin.NewTrustStore()
	trustPath := filepath.Join(".anyclaw", "trust.json")
	if err := trustStore.Load(trustPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load trust store: %w", err)
	}

	name := keyID
	if len(args) > 2 && strings.TrimSpace(args[2]) != "" {
		name = strings.TrimSpace(args[2])
	}
	trustStore.AddSigner(keyID, &plugin.SignerInfo{
		KeyID:      keyID,
		Name:       name,
		TrustLevel: plugin.TrustLevelTrusted,
		AddedAt:    time.Now().UTC(),
	})

	if err := os.MkdirAll(filepath.Dir(trustPath), 0o755); err != nil {
		return err
	}
	if err := trustStore.Save(trustPath); err != nil {
		return fmt.Errorf("failed to save trust store: %w", err)
	}

	printSuccess("Added %s to trusted signers!", keyID)
	return nil
}

func runStoreSources(args []string) error {
	sourcesPath := filepath.Join(".anyclaw", "sources.json")

	if len(args) > 0 && args[0] == "add" {
		if len(args) < 3 {
			return fmt.Errorf("usage: anyclaw store sources add <name> <url>")
		}

		sources, err := loadStoreSources(sourcesPath)
		if err != nil {
			return err
		}
		sources = append(sources, &plugin.PluginSource{
			Name: args[1],
			URL:  args[2],
			Type: "http",
		})
		if err := saveStoreSources(sourcesPath, sources); err != nil {
			return err
		}

		printSuccess("Added source: %s -> %s", args[1], args[2])
		return nil
	}

	sources, err := loadStoreSources(sourcesPath)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		printInfo("No sources configured.")
		return nil
	}

	fmt.Printf("Configured sources (%d):\n\n", len(sources))
	for _, source := range sources {
		fmt.Printf("  %s: %s (%s)\n", source.Name, source.URL, source.Type)
	}
	return nil
}

func loadStoreSources(path string) ([]*plugin.PluginSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read store sources: %w", err)
	}

	var sources []*plugin.PluginSource
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("failed to parse store sources: %w", err)
	}
	return sources, nil
}

func saveStoreSources(path string, sources []*plugin.PluginSource) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func runStoreUpdate(args []string) error {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		printInfo("Update for %s requires agentstore v2", args[0])
		return nil
	}
	printInfo("Update functionality requires agentstore v2")
	printInfo("Use 'anyclaw store install <plugin-id>' to update a plugin")
	return nil
}
