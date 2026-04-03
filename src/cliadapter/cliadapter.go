package cliadapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/aws"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/blender"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/curl"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/docker"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/ffmpeg"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/git"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/kubectl"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/libreoffice"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/node"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/npm"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/ollama"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/python"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/sqlite"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/zip"
	"github.com/anyclaw/anyclaw/pkg/cliadapter/adapters/zotero"
	ce "github.com/anyclaw/anyclaw/pkg/cliadapter/exec"
	cr "github.com/anyclaw/anyclaw/pkg/cliadapter/registry"
)

var defaultRegistry *cr.Registry
var defaultExecutor *ce.Executor

func Init(root string) error {
	reg, err := cr.NewRegistry(root)
	if err != nil {
		return err
	}

	defaultRegistry = reg
	defaultExecutor = ce.NewExecutor(reg)

	registerBuiltInHandlers()

	return nil
}

func InitFromEnv() error {
	roots := []string{
		os.Getenv("ANYCLAW_CLIADAPTER_ROOT"),
		"CLI-Anything-0.2.0",
		"../CLI-Anything-0.2.0",
		"../../CLI-Anything-0.2.0",
	}

	for _, root := range roots {
		if root == "" {
			continue
		}

		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}

		if _, err := os.Stat(filepath.Join(absRoot, "registry.json")); err == nil {
			return Init(absRoot)
		}
	}

	return nil
}

func GetRegistry() *cr.Registry {
	return defaultRegistry
}

func GetExecutor() *ce.Executor {
	return defaultExecutor
}

type SearchResult struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Installed   bool   `json:"installed"`
}

func Search(query, category string, limit int) ([]SearchResult, error) {
	if defaultExecutor == nil {
		return nil, nil
	}

	entries := defaultExecutor.Search(query, category, limit)
	results := make([]SearchResult, 0, len(entries))

	for _, e := range entries {
		results = append(results, SearchResult{
			Name:        e.Name,
			DisplayName: e.DisplayName,
			Category:    e.Category,
			Description: e.Description,
			Installed:   e.Installed,
		})

		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results, nil
}

func Exec(ctx context.Context, name string, args []string) (string, error) {
	if defaultExecutor == nil {
		return "", nil
	}

	result := defaultExecutor.Exec(ctx, name, args)
	if result.Error != "" {
		return result.Output, errors.New(result.Error)
	}
	return result.Output, nil
}

func ListCategories() map[string]int {
	if defaultExecutor == nil {
		return nil
	}
	return defaultExecutor.Categories()
}

func registerBuiltInHandlers() {
	if defaultExecutor == nil {
		return
	}

	defaultExecutor.RegisterHandler("echo", func(ctx context.Context, args []string) (string, error) {
		return strings.Join(args, " "), nil
	})

	defaultExecutor.RegisterHandler("date", func(ctx context.Context, args []string) (string, error) {
		return "2026-04-03", nil
	})

	defaultExecutor.RegisterHandler("pwd", func(ctx context.Context, args []string) (string, error) {
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return dir, nil
	})

	defaultExecutor.RegisterHandler("ollama", func(ctx context.Context, args []string) (string, error) {
		client := ollama.NewClient(ollama.Config{
			BaseURL: "http://localhost:11434",
			Model:   "llama2",
		})

		if len(args) == 0 {
			models, err := client.ListModels(ctx)
			if err != nil {
				return "", err
			}
			var out []string
			for _, m := range models {
				out = append(out, m.Name)
			}
			return strings.Join(out, "\n"), nil
		}

		switch args[0] {
		case "list":
			models, err := client.ListModels(ctx)
			if err != nil {
				return "", err
			}
			var out []string
			for _, m := range models {
				out = append(out, m.Name)
			}
			return strings.Join(out, "\n"), nil
		case "run":
			if len(args) < 2 {
				return "", fmt.Errorf("usage: ollama run <model> <prompt>")
			}
			client.Model = args[1]
			prompt := strings.Join(args[2:], " ")
			return client.Generate(ctx, prompt)
		default:
			return client.Generate(ctx, strings.Join(args, " "))
		}
	})

	defaultExecutor.RegisterHandler("zotero", func(ctx context.Context, args []string) (string, error) {
		client := zotero.NewClient(zotero.Config{
			LibraryPath: os.Getenv("ZOTERO_LIBRARY_PATH"),
		})

		if !client.IsConfigured() {
			return "", fmt.Errorf("Zotero not configured: set ZOTERO_API_KEY and ZOTERO_USER_ID")
		}

		if len(args) == 0 {
			items, err := client.ListItems(ctx, "", 10)
			if err != nil {
				return "", err
			}
			var out []string
			for _, item := range items {
				out = append(out, zotero.FormatItem(&item))
			}
			return strings.Join(out, "\n"), nil
		}

		switch args[0] {
		case "list":
			items, err := client.ListItems(ctx, "", 20)
			if err != nil {
				return "", err
			}
			var out []string
			for _, item := range items {
				out = append(out, zotero.FormatItem(&item))
			}
			return strings.Join(out, "\n"), nil
		case "search":
			if len(args) < 2 {
				return "", fmt.Errorf("usage: zotero search <query>")
			}
			items, err := client.Search(ctx, args[1], "", 10)
			if err != nil {
				return "", err
			}
			var out []string
			for _, item := range items {
				out = append(out, zotero.FormatItem(&item))
			}
			return strings.Join(out, "\n"), nil
		default:
			return "", fmt.Errorf("unknown command: %s", args[0])
		}
	})

	defaultExecutor.RegisterHandler("blender", func(ctx context.Context, args []string) (string, error) {
		client := blender.NewClient(blender.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("docker", func(ctx context.Context, args []string) (string, error) {
		client := docker.NewClient(docker.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("ffmpeg", func(ctx context.Context, args []string) (string, error) {
		client := ffmpeg.NewClient(ffmpeg.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("git", func(ctx context.Context, args []string) (string, error) {
		client := git.NewClient(git.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("libreoffice", func(ctx context.Context, args []string) (string, error) {
		client := libreoffice.NewClient(libreoffice.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("npm", func(ctx context.Context, args []string) (string, error) {
		client := npm.NewClient(npm.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("curl", func(ctx context.Context, args []string) (string, error) {
		client := curl.NewClient(curl.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("zip", func(ctx context.Context, args []string) (string, error) {
		client := zip.NewClient(zip.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("kubectl", func(ctx context.Context, args []string) (string, error) {
		client := kubectl.NewClient(kubectl.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("aws", func(ctx context.Context, args []string) (string, error) {
		client := aws.NewClient(aws.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("sqlite", func(ctx context.Context, args []string) (string, error) {
		client := sqlite.NewClient(sqlite.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("python", func(ctx context.Context, args []string) (string, error) {
		client := python.NewClient(python.Config{})
		return client.Run(ctx, args)
	})

	defaultExecutor.RegisterHandler("node", func(ctx context.Context, args []string) (string, error) {
		client := node.NewClient(node.Config{})
		return client.Run(ctx, args)
	})
}

func DiscoverRoot(start string) (string, bool) {
	candidates := []string{
		start,
		filepath.Join(start, "CLI-Anything-0.2.0"),
		filepath.Join(start, "CLI-Anything"),
		"..",
		"../..",
		"../../..",
	}

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}

		if _, err := os.Stat(filepath.Join(abs, "registry.json")); err == nil {
			return abs, true
		}
	}

	return "", false
}
