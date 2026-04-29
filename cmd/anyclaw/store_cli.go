package main

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentstore "github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
)

type storeCommandOptions struct {
	configPath string
	workDir    string
}

func runStoreCommand(args []string) error {
	opts, args, err := parseStoreCommandOptions(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		printStoreUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		return runStoreList(args[1:], opts)
	case "search":
		return runStoreSearch(args[1:], opts)
	case "info":
		return runStoreInfo(args[1:], opts)
	case "install":
		return runStoreInstall(args[1:], opts)
	case "uninstall":
		return runStoreUninstall(args[1:], opts)
	case "sign":
		return runStoreSign(args[1:], opts)
	case "verify":
		return runStoreVerify(args[1:], opts)
	case "trust":
		return runStoreTrust(args[1:], opts)
	case "sources":
		return runStoreSources(args[1:], opts)
	case "update":
		return runStoreUpdate(args[1:], opts)
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

Options:
  --config <path>  Path to anyclaw.json
`)
}

func parseStoreCommandOptions(args []string) (storeCommandOptions, []string, error) {
	opts := storeCommandOptions{configPath: "anyclaw.json"}
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return opts, nil, fmt.Errorf("--config requires a path")
			}
			opts.configPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			if value == "" {
				return opts, nil, fmt.Errorf("--config requires a path")
			}
			opts.configPath = value
		default:
			remaining = append(remaining, arg)
		}
	}
	return opts, remaining, nil
}

func newStoreManager(opts storeCommandOptions) (agentstore.StoreManager, error) {
	resolved, err := resolveStoreCommandOptions(opts)
	if err != nil {
		return nil, err
	}
	return agentstore.NewStoreManager(resolved.workDir, resolved.configPath)
}

func resolveStoreCommandOptions(opts storeCommandOptions) (storeCommandOptions, error) {
	configPath := strings.TrimSpace(opts.configPath)
	if configPath == "" {
		configPath = "anyclaw.json"
	}
	resolvedConfigPath := config.ResolveConfigPath(configPath)

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return storeCommandOptions{}, err
	}
	workDir := strings.TrimSpace(opts.workDir)
	if workDir == "" {
		workDir = cfg.Agent.WorkDir
	}
	if strings.TrimSpace(workDir) == "" {
		workDir = ".anyclaw"
	}
	return storeCommandOptions{
		configPath: resolvedConfigPath,
		workDir:    config.ResolvePath(resolvedConfigPath, workDir),
	}, nil
}

func runStoreList(args []string, opts storeCommandOptions) error {
	sm, err := newStoreManager(opts)
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

func runStoreSearch(args []string, opts storeCommandOptions) error {
	keyword := strings.TrimSpace(strings.Join(args, " "))
	if keyword == "" {
		return fmt.Errorf("usage: anyclaw store search <keyword>")
	}

	sm, err := newStoreManager(opts)
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

func runStoreInfo(args []string, opts storeCommandOptions) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: anyclaw store info <id>")
	}

	sm, err := newStoreManager(opts)
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

func runStoreInstall(args []string, opts storeCommandOptions) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: anyclaw store install <id>")
	}

	sm, err := newStoreManager(opts)
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

func runStoreUninstall(args []string, opts storeCommandOptions) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: anyclaw store uninstall <id>")
	}

	sm, err := newStoreManager(opts)
	if err != nil {
		return err
	}

	if err := sm.Uninstall(args[0]); err != nil {
		return err
	}
	printSuccess("Uninstalled: %s", args[0])
	return nil
}

func runStoreSign(args []string, opts storeCommandOptions) error {
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

func runStoreVerify(args []string, opts storeCommandOptions) error {
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

func runStoreTrust(args []string, opts storeCommandOptions) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: anyclaw store trust <key-id> <public-key-file> [name]")
	}

	keyID := strings.TrimSpace(args[0])
	if keyID == "" {
		return fmt.Errorf("key id is required")
	}
	keyData, err := os.ReadFile(args[1])
	if err != nil {
		return fmt.Errorf("failed to read public key file: %w", err)
	}
	derivedKeyID, fingerprint, err := publicKeyIdentity(keyData)
	if err != nil {
		return fmt.Errorf("failed to parse public key file: %w", err)
	}
	if !strings.EqualFold(keyID, derivedKeyID) {
		return fmt.Errorf("key id %s does not match public key %s", keyID, derivedKeyID)
	}

	resolved, err := resolveStoreCommandOptions(opts)
	if err != nil {
		return err
	}
	trustStore := plugin.NewTrustStore()
	trustPath := filepath.Join(resolved.workDir, "trust.json")
	if err := trustStore.Load(trustPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load trust store: %w", err)
	}

	name := keyID
	if len(args) > 2 && strings.TrimSpace(args[2]) != "" {
		name = strings.TrimSpace(args[2])
	}
	trustStore.AddSigner(keyID, &plugin.SignerInfo{
		KeyID:       derivedKeyID,
		Name:        name,
		Fingerprint: fingerprint,
		TrustLevel:  plugin.TrustLevelTrusted,
		AddedAt:     time.Now().UTC(),
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

func publicKeyIdentity(data []byte) (string, string, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return "", "", fmt.Errorf("failed to decode public key")
	}
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", "", err
	}
	if _, ok := publicKey.(*rsa.PublicKey); !ok {
		return "", "", fmt.Errorf("not an RSA public key")
	}
	sum := sha256.Sum256(block.Bytes)
	fingerprint := fmt.Sprintf("%x", sum)
	return fingerprint[:16], fingerprint, nil
}

func runStoreSources(args []string, opts storeCommandOptions) error {
	resolved, err := resolveStoreCommandOptions(opts)
	if err != nil {
		return err
	}
	sourcesPath := filepath.Join(resolved.workDir, "sources.json")

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

func runStoreUpdate(args []string, opts storeCommandOptions) error {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		printInfo("Update for %s requires agentstore v2", args[0])
		return nil
	}
	printInfo("Update functionality requires agentstore v2")
	printInfo("Use 'anyclaw store install <plugin-id>' to update a plugin")
	return nil
}
