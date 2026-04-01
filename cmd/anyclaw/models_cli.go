package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type modelProviderView struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Enabled      bool   `json:"enabled"`
	IsDefault    bool   `json:"is_default"`
	DefaultModel string `json:"default_model,omitempty"`
	HasAPIKey    bool   `json:"has_api_key"`
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
}

func runModelsCommand(args []string) error {
	if len(args) == 0 {
		return runModelsStatus(nil)
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "status":
		return runModelsStatus(args[1:])
	case "list":
		return runModelsList(args[1:])
	case "set":
		return runModelsSet(args[1:])
	default:
		printModelsUsage()
		return fmt.Errorf("unknown models command: %s", args[0])
	}
}

func runModelsStatus(args []string) error {
	fs := flag.NewFlagSet("models status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	views := localModelProviderViews(cfg)
	defaultProvider, hasDefault := cfg.FindDefaultProviderProfile()

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"current_provider": cfg.LLM.Provider,
			"current_model":    cfg.LLM.Model,
			"default_provider": defaultProvider.ID,
			"has_default":      hasDefault,
			"providers":        views,
		})
	}

	printSuccess("Current model: %s", cfg.LLM.Model)
	fmt.Printf("Provider: %s\n", cfg.LLM.Provider)
	if hasDefault {
		fmt.Printf("Default provider: %s (%s)\n", defaultProvider.Name, defaultProvider.ID)
	}
	if len(views) == 0 {
		printInfo("No provider profiles configured")
		return nil
	}

	fmt.Println()
	fmt.Println("Configured providers:")
	for _, item := range views {
		label := item.Name
		if item.ID != "" && !strings.EqualFold(item.ID, item.Name) {
			label += " (" + item.ID + ")"
		}
		suffix := ""
		if item.IsDefault {
			suffix = " [default]"
		}
		fmt.Printf("  - %s%s\n", label, suffix)
		fmt.Printf("    runtime=%s model=%s status=%s\n", item.Provider, firstNonEmptyModel(item.DefaultModel, cfg.LLM.Model), item.Status)
		if strings.TrimSpace(item.Message) != "" {
			fmt.Printf("    note=%s\n", item.Message)
		}
	}
	return nil
}

func runModelsList(args []string) error {
	fs := flag.NewFlagSet("models list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	providerFilter := fs.String("provider", "", "filter by provider runtime or provider profile id")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	catalog := builtinModelCatalog()
	if strings.TrimSpace(*providerFilter) != "" {
		runtime := resolveModelRuntime(cfg, *providerFilter)
		models := mergeProviderModels(runtime, cfg, catalog[runtime])
		if *jsonOut {
			return writePrettyJSON(map[string]any{
				"provider": runtime,
				"models":   models,
			})
		}
		printSuccess("Models for %s", runtime)
		for _, model := range models {
			fmt.Printf("  - %s\n", model)
		}
		return nil
	}

	keys := make([]string, 0, len(catalog))
	for key := range catalog {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	payload := map[string]any{
		"current_provider": cfg.LLM.Provider,
		"current_model":    cfg.LLM.Model,
		"catalog":          map[string][]string{},
	}
	catalogPayload := payload["catalog"].(map[string][]string)
	for _, key := range keys {
		catalogPayload[key] = mergeProviderModels(key, cfg, catalog[key])
	}
	if *jsonOut {
		return writePrettyJSON(payload)
	}

	for _, key := range keys {
		fmt.Println(key)
		for _, model := range catalogPayload[key] {
			fmt.Printf("  - %s\n", model)
		}
		fmt.Println()
	}
	return nil
}

func runModelsSet(args []string) error {
	fs := flag.NewFlagSet("models set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: anyclaw models set <model>")
	}

	model := strings.TrimSpace(fs.Arg(0))
	if model == "" {
		return fmt.Errorf("model is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	cfg.LLM.Model = model

	if current, ok := cfg.FindDefaultProviderProfile(); ok {
		current.DefaultModel = model
		if err := cfg.UpsertProviderProfile(current); err != nil {
			return err
		}
	}

	if err := cfg.Save(*configPath); err != nil {
		return err
	}
	printSuccess("Default model set to %s", model)
	return nil
}

func printModelsUsage() {
	fmt.Print(`AnyClaw models commands:

Usage:
  anyclaw models
  anyclaw models status [--json]
  anyclaw models list [--provider openai] [--json]
  anyclaw models set <model>
`)
}

func builtinModelCatalog() map[string][]string {
	return map[string][]string{
		"anthropic": {
			"claude-opus-4-5",
			"claude-sonnet-4-7",
			"claude-haiku-3-5",
		},
		"compatible": {
			"(use your provider's model names)",
		},
		"ollama": {
			"llama3.2",
			"llama3.1",
			"codellama",
			"mistral",
			"qwen2.5",
		},
		"openai": {
			"gpt-4o",
			"gpt-4o-mini",
			"gpt-4-turbo",
			"gpt-4",
			"gpt-3.5-turbo",
		},
		"qwen": {
			"qwen-plus",
			"qwen-turbo",
			"qwen-max",
			"qwen2.5-72b-instruct",
			"qwen2.5-14b-instruct",
			"qwq-32b-preview",
			"qwen-coder-plus",
		},
	}
}

func resolveModelRuntime(cfg *config.Config, filter string) string {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return strings.TrimSpace(strings.ToLower(cfg.LLM.Provider))
	}
	if provider, ok := cfg.FindProviderProfile(filter); ok {
		return strings.TrimSpace(strings.ToLower(provider.Provider))
	}
	return filter
}

func mergeProviderModels(runtime string, cfg *config.Config, base []string) []string {
	items := make([]string, 0, len(base)+2)
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			return
		}
		seen[strings.ToLower(value)] = true
		items = append(items, value)
	}

	for _, model := range base {
		add(model)
	}
	for _, provider := range cfg.Providers {
		if strings.EqualFold(strings.TrimSpace(provider.Provider), runtime) {
			add(provider.DefaultModel)
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.LLM.Provider), runtime) {
		add(cfg.LLM.Model)
	}
	return items
}

func localModelProviderViews(cfg *config.Config) []modelProviderView {
	items := make([]modelProviderView, 0, len(cfg.Providers))
	defaultRef := strings.TrimSpace(cfg.LLM.DefaultProviderRef)
	for _, provider := range cfg.Providers {
		status, message := localProviderHealth(provider)
		items = append(items, modelProviderView{
			ID:           provider.ID,
			Name:         provider.Name,
			Provider:     provider.Provider,
			Enabled:      provider.IsEnabled(),
			IsDefault:    strings.EqualFold(defaultRef, provider.ID),
			DefaultModel: provider.DefaultModel,
			HasAPIKey:    strings.TrimSpace(provider.APIKey) != "",
			Status:       status,
			Message:      message,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items
}

func localProviderHealth(provider config.ProviderProfile) (string, string) {
	if !provider.IsEnabled() {
		return "disabled", "provider is disabled"
	}
	if strings.TrimSpace(provider.Provider) == "" {
		return "invalid", "missing runtime provider"
	}
	if providerNeedsAPIKey(provider.Provider) && strings.TrimSpace(provider.APIKey) == "" {
		return "missing_key", "API key required"
	}
	if base := strings.TrimSpace(provider.BaseURL); base != "" {
		if _, err := url.ParseRequestURI(base); err != nil {
			return "invalid_base_url", "base URL is invalid"
		}
	}
	return "ready", "ready to use"
}

func providerNeedsAPIKey(provider string) bool {
	return !strings.EqualFold(strings.TrimSpace(provider), "ollama")
}

func firstNonEmptyModel(primary string, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}
