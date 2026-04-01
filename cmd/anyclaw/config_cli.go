package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type configPathToken struct {
	Key     string
	Index   int
	IsIndex bool
}

func runConfigCommand(args []string) error {
	if len(args) == 0 {
		printConfigUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "get":
		return runConfigGet(args[1:])
	case "set":
		return runConfigSet(args[1:])
	case "unset":
		return runConfigUnset(args[1:])
	case "file":
		return runConfigFile(args[1:])
	case "validate":
		return runConfigValidate(args[1:])
	default:
		printConfigUsage()
		return fmt.Errorf("unknown config command: %s", args[0])
	}
}

func runConfigGet(args []string) error {
	fs := flag.NewFlagSet("config get", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("config path is required")
	}

	tokens, err := parseConfigPath(fs.Arg(0))
	if err != nil {
		return err
	}
	doc, err := loadEffectiveConfigDocument(*configPath)
	if err != nil {
		return err
	}
	value, ok, err := lookupConfigPath(doc, tokens)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("config path not found: %s", fs.Arg(0))
	}
	return writeConfigValue(value)
}

func runConfigSet(args []string) error {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: anyclaw config set <path> <value>")
	}

	pathExpr := fs.Arg(0)
	valueExpr := fs.Arg(1)
	tokens, err := parseConfigPath(pathExpr)
	if err != nil {
		return err
	}
	doc, err := loadRawConfigDocument(*configPath)
	if err != nil {
		return err
	}
	updated, err := setConfigPath(doc, tokens, parseConfigValue(valueExpr))
	if err != nil {
		return err
	}
	root, ok := updated.(map[string]any)
	if !ok {
		return fmt.Errorf("config root must remain an object")
	}
	if err := validateConfigDocument(root); err != nil {
		return err
	}
	if err := saveConfigDocument(*configPath, root); err != nil {
		return err
	}
	printSuccess("Updated config: %s", pathExpr)
	return nil
}

func runConfigUnset(args []string) error {
	fs := flag.NewFlagSet("config unset", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: anyclaw config unset <path>")
	}

	pathExpr := fs.Arg(0)
	tokens, err := parseConfigPath(pathExpr)
	if err != nil {
		return err
	}
	doc, err := loadRawConfigDocument(*configPath)
	if err != nil {
		return err
	}
	updated, removed, err := unsetConfigPath(doc, tokens)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("config path not found: %s", pathExpr)
	}
	root, ok := updated.(map[string]any)
	if !ok {
		return fmt.Errorf("config root must remain an object")
	}
	if err := validateConfigDocument(root); err != nil {
		return err
	}
	if err := saveConfigDocument(*configPath, root); err != nil {
		return err
	}
	printSuccess("Unset config: %s", pathExpr)
	return nil
}

func runConfigFile(args []string) error {
	fs := flag.NewFlagSet("config file", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Println(config.ResolveConfigPath(*configPath))
	return nil
}

func runConfigValidate(args []string) error {
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	doc, err := loadRawConfigDocument(*configPath)
	if err != nil {
		if *jsonOut {
			_ = writePrettyJSON(map[string]any{
				"ok":    false,
				"path":  config.ResolveConfigPath(*configPath),
				"error": err.Error(),
			})
		}
		return err
	}
	if err := validateConfigDocument(doc); err != nil {
		if *jsonOut {
			_ = writePrettyJSON(map[string]any{
				"ok":    false,
				"path":  config.ResolveConfigPath(*configPath),
				"error": err.Error(),
			})
		}
		return err
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"ok":   true,
			"path": config.ResolveConfigPath(*configPath),
		})
	}

	printSuccess("Config valid: %s", config.ResolveConfigPath(*configPath))
	return nil
}

func printConfigUsage() {
	fmt.Print(`AnyClaw config commands:

Usage:
  anyclaw config get <path>
  anyclaw config set <path> <value>
  anyclaw config unset <path>
  anyclaw config file
  anyclaw config validate [--json]
`)
}

func loadRawConfigDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func loadEffectiveConfigDocument(path string) (map[string]any, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func saveConfigDocument(path string, doc map[string]any) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func validateConfigDocument(doc map[string]any) error {
	cfg := config.DefaultConfig()
	if len(doc) > 0 {
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return fmt.Errorf("config parse failed: %w", err)
		}
	}
	return cfg.Validate()
}

func writeConfigValue(value any) error {
	switch typed := value.(type) {
	case nil:
		fmt.Println("null")
		return nil
	case string:
		fmt.Println(typed)
		return nil
	case bool:
		if typed {
			fmt.Println("true")
		} else {
			fmt.Println("false")
		}
		return nil
	case float64, []any, map[string]any:
		return writePrettyJSON(typed)
	default:
		return writePrettyJSON(typed)
	}
}

func parseConfigValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}
	return raw
}

func parseConfigPath(path string) ([]configPathToken, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("config path is required")
	}
	tokens := make([]configPathToken, 0)
	for i := 0; i < len(path); {
		switch path[i] {
		case '.':
			i++
		case '[':
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unterminated index in path: %s", path)
			}
			indexText := strings.TrimSpace(path[i+1 : i+end])
			index, err := strconv.Atoi(indexText)
			if err != nil || index < 0 {
				return nil, fmt.Errorf("invalid index %q in path: %s", indexText, path)
			}
			tokens = append(tokens, configPathToken{Index: index, IsIndex: true})
			i += end + 1
		default:
			start := i
			for i < len(path) && path[i] != '.' && path[i] != '[' {
				i++
			}
			key := strings.TrimSpace(path[start:i])
			if key == "" {
				return nil, fmt.Errorf("invalid config path: %s", path)
			}
			tokens = append(tokens, configPathToken{Key: key})
		}
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("config path is required")
	}
	return tokens, nil
}

func lookupConfigPath(current any, tokens []configPathToken) (any, bool, error) {
	for _, token := range tokens {
		if token.IsIndex {
			items, ok := current.([]any)
			if !ok {
				return nil, false, fmt.Errorf("path segment is not an array index")
			}
			if token.Index < 0 || token.Index >= len(items) {
				return nil, false, nil
			}
			current = items[token.Index]
			continue
		}
		items, ok := current.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("path segment is not an object key")
		}
		next, ok := items[token.Key]
		if !ok {
			return nil, false, nil
		}
		current = next
	}
	return current, true, nil
}

func setConfigPath(current any, tokens []configPathToken, value any) (any, error) {
	if len(tokens) == 0 {
		return value, nil
	}
	token := tokens[0]
	if token.IsIndex {
		var items []any
		if current == nil {
			items = []any{}
		} else {
			typed, ok := current.([]any)
			if !ok {
				return nil, fmt.Errorf("path segment is not an array index")
			}
			items = typed
		}
		for len(items) <= token.Index {
			items = append(items, nil)
		}
		next, err := setConfigPath(items[token.Index], tokens[1:], value)
		if err != nil {
			return nil, err
		}
		items[token.Index] = next
		return items, nil
	}

	var items map[string]any
	if current == nil {
		items = map[string]any{}
	} else {
		typed, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path segment is not an object key")
		}
		items = typed
	}
	next, err := setConfigPath(items[token.Key], tokens[1:], value)
	if err != nil {
		return nil, err
	}
	items[token.Key] = next
	return items, nil
}

func unsetConfigPath(current any, tokens []configPathToken) (any, bool, error) {
	if len(tokens) == 0 {
		return current, false, nil
	}
	token := tokens[0]

	if len(tokens) == 1 {
		if token.IsIndex {
			items, ok := current.([]any)
			if !ok {
				return nil, false, fmt.Errorf("path segment is not an array index")
			}
			if token.Index < 0 || token.Index >= len(items) {
				return items, false, nil
			}
			items = append(items[:token.Index], items[token.Index+1:]...)
			return items, true, nil
		}
		items, ok := current.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("path segment is not an object key")
		}
		if _, exists := items[token.Key]; !exists {
			return items, false, nil
		}
		delete(items, token.Key)
		return items, true, nil
	}

	if token.IsIndex {
		items, ok := current.([]any)
		if !ok {
			return nil, false, fmt.Errorf("path segment is not an array index")
		}
		if token.Index < 0 || token.Index >= len(items) {
			return items, false, nil
		}
		next, removed, err := unsetConfigPath(items[token.Index], tokens[1:])
		if err != nil {
			return nil, false, err
		}
		if !removed {
			return items, false, nil
		}
		items[token.Index] = next
		return items, true, nil
	}

	items, ok := current.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("path segment is not an object key")
	}
	child, exists := items[token.Key]
	if !exists {
		return items, false, nil
	}
	next, removed, err := unsetConfigPath(child, tokens[1:])
	if err != nil {
		return nil, false, err
	}
	if !removed {
		return items, false, nil
	}
	items[token.Key] = next
	return items, true, nil
}
