package mubu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Adapter struct {
	dataDir string
}

type Config struct {
	DataDir string
}

func New(cfg Config) *Adapter {
	dir := cfg.DataDir
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Library", "Application Support", "Mubu")
	}
	return &Adapter{dataDir: dir}
}

func (a *Adapter) Name() string {
	return "mubu"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.list()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "list":
		return a.list()
	case "documents":
		return a.documents()
	case "search":
		return a.search(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) list() (string, error) {
	entries, err := os.ReadDir(a.dataDir)
	if err != nil {
		return "", fmt.Errorf("cannot read mubu data: %w", err)
	}

	var result []string
	for _, e := range entries {
		if e.IsDir() {
			result = append(result, e.Name())
		}
	}
	if len(result) == 0 {
		return "No documents found", nil
	}
	return strings.Join(result, "\n"), nil
}

func (a *Adapter) documents() (string, error) {
	return a.list()
}

func (a *Adapter) search(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("search requires <query>")
	}
	query := strings.Join(args, " ")

	entries, err := os.ReadDir(a.dataDir)
	if err != nil {
		return "", err
	}

	var result []string
	for _, e := range entries {
		if e.IsDir() && strings.Contains(strings.ToLower(e.Name()), strings.ToLower(query)) {
			result = append(result, e.Name())
		}
	}

	if len(result) == 0 {
		return fmt.Sprintf("No documents matching '%s'", query), nil
	}
	return strings.Join(result, "\n"), nil
}

func (a *Adapter) help() (string, error) {
	return `Mubu CLI adapter (knowledge management)
Commands:
  list                    - List documents
  documents               - List documents (alias)
  search <query>          - Search documents
  help                    - Show this help`, nil
}
